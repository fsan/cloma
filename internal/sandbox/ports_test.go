package sandbox

import (
	"fmt"
	"net"
	"testing"
)

// TestFreeLoopbackPortSkipsBusyPort verifies that FreeLoopbackPort does not
// hand back a port that is already in use, and that the port it returns can
// actually be bound.
func TestFreeLoopbackPortSkipsBusyPort(t *testing.T) {
	// Occupy a port to force the scan to move on.
	busy, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to occupy a port: %v", err)
	}
	defer busy.Close()
	busyPort := busy.Addr().(*net.TCPAddr).Port

	got := FreeLoopbackPort(busyPort)
	if got == busyPort {
		t.Fatalf("FreeLoopbackPort(%d) returned the busy port", busyPort)
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", got))
	if err != nil {
		t.Fatalf("FreeLoopbackPort returned %d, which is not bindable: %v", got, err)
	}
	ln.Close()
}
