package sandbox

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

// sandboxSSHSuffix is the DNS suffix Docker sandboxes answer on. `sbx setup
// ssh` (or the Docker Desktop sandbox integration) writes a `Host *.sbx`
// block into ~/.ssh/config whose ProxyCommand relays the stream to the
// sandbox daemon over its local socket, so `<name>.sbx` behaves like a normal
// SSH host. Authentication is tied to the host login, not a stored key.
const sandboxSSHSuffix = ".sbx"

// DefaultDshWebPort is the port the DeepSeek Harness web UI listens on inside
// the sandbox. dsh binds it to the sandbox loopback (it rejects 0.0.0.0), so
// it cannot be reached with `sbx port publish`; an SSH local forward — which
// the sandbox daemon accepts to loopback addresses — is the transport.
const DefaultDshWebPort = 3080

// SSHForward is a running `ssh -N -L` tunnel that exposes a loopback service
// from inside a sandbox on the host loopback.
type SSHForward struct {
	cmd      *exec.Cmd
	done     chan struct{}
	HostPort int
}

// SandboxSSHAvailable reports whether the standalone `sbx` CLI is installed.
func SandboxSSHAvailable() bool {
	_, err := exec.LookPath("sbx")
	return err == nil
}

// ErrSandboxSSHUnsupported indicates the installed sbx CLI has no `setup ssh`
// subcommand. That command was added in sbx v0.37.0; older versions must use
// `sbx ports` instead (which requires the in-sandbox service to bind 0.0.0.0).
var ErrSandboxSSHUnsupported = errors.New("the installed sbx CLI has no `setup ssh` command (requires sbx v0.37.0 or newer)")

// SetupSandboxSSH runs `sbx setup ssh`, which writes the managed `Host *.sbx`
// block into ~/.ssh/config so that `<name>.sbx` resolves through the sandbox
// daemon. The command is idempotent. When the `sbx` CLI is not installed this
// is a no-op returning nil, so callers fall back to surfacing the resolver
// error from the forward itself. It returns ErrSandboxSSHUnsupported when the
// subcommand is unknown to the installed version.
func SetupSandboxSSH() error {
	if !SandboxSSHAvailable() {
		return nil
	}
	cmd := exec.Command("sbx", "setup", "ssh")
	output, err := cmd.CombinedOutput()
	if err != nil {
		if strings.Contains(string(output), "unknown command") {
			return ErrSandboxSSHUnsupported
		}
		return CommandError("`sbx setup ssh` failed", err, output)
	}
	return nil
}

// FreeLoopbackPort returns the first free TCP port on 127.0.0.1 at or above
// preferred, scanning up to 100 ports. If none is free it returns preferred,
// letting the caller surface the bind failure rather than guessing further.
func FreeLoopbackPort(preferred int) int {
	for p := preferred; p < preferred+100 && p < 65536; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			ln.Close()
			return p
		}
	}
	return preferred
}

// StartSSHForward starts an SSH local port forward that maps the host's
// 127.0.0.1:hostPort to 127.0.0.1:remotePort inside the sandbox, and returns
// once the host port is accepting connections. The returned handle must be
// closed with Stop when the session ends.
func StartSSHForward(sandboxName string, hostPort, remotePort int) (*SSHForward, error) {
	if _, err := exec.LookPath("ssh"); err != nil {
		return nil, fmt.Errorf("ssh is not installed or not in PATH")
	}

	host := sandboxName + sandboxSSHSuffix
	args := []string{
		"-N",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "BatchMode=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-L", fmt.Sprintf("127.0.0.1:%d:127.0.0.1:%d", hostPort, remotePort),
		host,
	}

	cmd := exec.Command("ssh", args...)
	cmd.Stderr = os.Stderr

	fwd := &SSHForward{cmd: cmd, done: make(chan struct{}), HostPort: hostPort}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start ssh forward to %s: %w", host, err)
	}
	go func() {
		cmd.Wait()
		close(fwd.done)
	}()

	deadline := time.Now().Add(10 * time.Second)
	for {
		select {
		case <-fwd.done:
			if SandboxSSHAvailable() {
				return nil, fmt.Errorf("ssh forward to %s exited before the port opened; check that the sandbox is running and that `sbx setup ssh` completed", host)
			}
			return nil, fmt.Errorf("ssh forward to %s exited before the port opened; the %q SSH alias is not configured — install the `sbx` CLI and run `sbx setup ssh` (or use the Docker Desktop sandbox SSH integration) so sandbox names resolve as SSH hosts", host, sandboxSSHSuffix)
		default:
		}
		if portOpen(hostPort) {
			return fwd, nil
		}
		if time.Now().After(deadline) {
			cmd.Process.Kill()
			return nil, fmt.Errorf("ssh forward did not open 127.0.0.1:%d within 10s", hostPort)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// Stop terminates the SSH forward. It is safe to call on a nil handle and
// blocks until the ssh process has been reaped.
func (f *SSHForward) Stop() {
	if f == nil || f.cmd == nil || f.cmd.Process == nil {
		return
	}
	f.cmd.Process.Kill()
	<-f.done
}

// portOpen reports whether something is accepting TCP connections on the
// given loopback port.
func portOpen(port int) bool {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 500*time.Millisecond)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}
