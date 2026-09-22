package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writePolicyFile writes content to a temp file and returns its path.
func writePolicyFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "policy.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write policy file: %v", err)
	}
	return path
}

func TestLoadNetworkPolicy(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name: "valid allow and block",
			yaml: `
allow:
  host_ports: [8881, 3000]
  domains: [api.github.com]
block:
  host_ports: [11434]
  domains: [example.com]
`,
		},
		{
			name: "empty file",
			yaml: ``,
		},
		{
			name: "only host ports",
			yaml: `
allow:
  host_ports:
    - 8881
`,
		},
		{
			name: "unknown key rejected",
			yaml: `
allow:
  host_port: [8881]
`,
			wantErr: "host_port",
		},
		{
			name: "port out of range",
			yaml: `
allow:
  host_ports: [0]
`,
			wantErr: "out of range",
		},
		{
			name: "port too large",
			yaml: `
allow:
  host_ports: [70000]
`,
			wantErr: "out of range",
		},
		{
			name: "localhost in domains rejected",
			yaml: `
allow:
  domains: [localhost]
`,
			wantErr: "host_ports instead",
		},
		{
			name: "host.docker.internal in domains rejected",
			yaml: `
allow:
  domains: [host.docker.internal]
`,
			wantErr: "host_ports instead",
		},
		{
			name: "scheme in domain rejected",
			yaml: `
allow:
  domains: [https://api.github.com]
`,
			wantErr: "not a bare hostname",
		},
		{
			name: "allow block port conflict",
			yaml: `
allow:
  host_ports: [8881]
block:
  host_ports: [8881]
`,
			wantErr: "both allow.host_ports and block.host_ports",
		},
		{
			name: "allow block domain conflict",
			yaml: `
allow:
  domains: [example.com]
block:
  domains: [example.com]
`,
			wantErr: "both allow.domains and block.domains",
		},
		{
			name: "malformed yaml",
			yaml: `
allow: [
`,
			wantErr: "failed to parse",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writePolicyFile(t, tt.yaml)
			policy, err := LoadNetworkPolicy(path)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("LoadNetworkPolicy(%q) unexpected error: %v", tt.yaml, err)
				}
				if policy == nil {
					t.Fatal("LoadNetworkPolicy returned nil policy without error")
				}
			} else {
				if err == nil {
					t.Fatalf("LoadNetworkPolicy(%q) expected error containing %q, got nil", tt.yaml, tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("LoadNetworkPolicy(%q) error %q does not contain %q", tt.yaml, err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestLoadNetworkPolicyMissingFile(t *testing.T) {
	_, err := LoadNetworkPolicy(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("expected error for missing policy file")
	}
}

func TestNetworkPolicySummary(t *testing.T) {
	empty := &NetworkPolicy{}
	if got := empty.Summary(); !strings.Contains(got, "no entries") {
		t.Fatalf("Summary() for empty policy = %q, want it to mention no entries", got)
	}

	full := &NetworkPolicy{
		Allow: &PolicyAllow{HostPorts: []int{8881}, Domains: []string{"api.github.com"}},
		Block: &PolicyBlock{HostPorts: []int{11434}, Domains: []string{"example.com"}},
	}
	got := full.Summary()
	for _, want := range []string{"allow host ports [8881]", "allow domains [api.github.com]", "block host ports [11434]", "block domains [example.com]"} {
		if !strings.Contains(got, want) {
			t.Errorf("Summary() = %q, missing %q", got, want)
		}
	}
}

func TestNetworkPolicyEffective(t *testing.T) {
	// A nil policy yields the default: only the Ollama port allowed.
	var nilPolicy *NetworkPolicy
	got := nilPolicy.Effective(11434)
	if len(got.Allow.HostPorts) != 1 || got.Allow.HostPorts[0] != 11434 {
		t.Fatalf("Effective() on nil policy = %+v, want host ports [11434]", got.Allow)
	}
	if got.Block != nil {
		t.Fatalf("Effective() on nil policy has block section %+v, want nil", got.Block)
	}

	// A full policy: default Ollama allow merges with allow entries, block
	// wins, and duplicates of the Ollama port are collapsed.
	policy := &NetworkPolicy{
		Allow: &PolicyAllow{
			HostPorts: []int{8881, 3000, 11434},
			Domains:   []string{"api.github.com", "example.com"},
		},
		Block: &PolicyBlock{
			HostPorts: []int{9999},
			Domains:   []string{"example.com"},
		},
	}
	// Note: example.com in both allow and block would be rejected by
	// validate() at load time; Effective is exercised directly here to
	// verify block-wins filtering regardless.
	got = policy.Effective(11434)
	wantPorts := []int{11434, 8881, 3000}
	if len(got.Allow.HostPorts) != len(wantPorts) {
		t.Fatalf("Effective() host ports = %v, want %v", got.Allow.HostPorts, wantPorts)
	}
	for i, port := range wantPorts {
		if got.Allow.HostPorts[i] != port {
			t.Errorf("Effective() host ports = %v, want %v", got.Allow.HostPorts, wantPorts)
			break
		}
	}
	wantDomains := []string{"api.github.com"}
	if len(got.Allow.Domains) != len(wantDomains) || got.Allow.Domains[0] != wantDomains[0] {
		t.Errorf("Effective() domains = %v, want %v (blocked example.com filtered)", got.Allow.Domains, wantDomains)
	}
	if len(got.Block.HostPorts) != 1 || got.Block.HostPorts[0] != 9999 {
		t.Errorf("Effective() kept block section = %+v, want host ports [9999]", got.Block)
	}

	// Blocking the Ollama port revokes the default allow.
	strict := &NetworkPolicy{Block: &PolicyBlock{HostPorts: []int{11434}}}
	got = strict.Effective(11434)
	if len(got.Allow.HostPorts) != 0 {
		t.Errorf("Effective() with blocked Ollama port = %v, want no allowed ports", got.Allow.HostPorts)
	}
}

func TestStoreAndGetNetworkPolicy(t *testing.T) {
	_, cleanup := withTempHome(t)
	defer cleanup()

	const name = "cloma-policy-test"

	// No record yet.
	policy, err := GetStoredNetworkPolicy(name)
	if err != nil || policy != nil {
		t.Fatalf("GetStoredNetworkPolicy on missing record = (%+v, %v), want (nil, nil)", policy, err)
	}

	// Store workspace + agent first (as a launch would), then the policy.
	if err := StoreMetadata(name, "/tmp/ws", "claude"); err != nil {
		t.Fatalf("StoreMetadata: %v", err)
	}
	want := &NetworkPolicy{
		Allow: &PolicyAllow{HostPorts: []int{11434, 8881}, Domains: []string{"api.github.com"}},
		Block: &PolicyBlock{HostPorts: []int{9999}},
	}
	if err := StoreNetworkPolicy(name, want); err != nil {
		t.Fatalf("StoreNetworkPolicy: %v", err)
	}

	got, err := GetStoredNetworkPolicy(name)
	if err != nil {
		t.Fatalf("GetStoredNetworkPolicy: %v", err)
	}
	if len(got.Allow.HostPorts) != 2 || got.Allow.HostPorts[0] != 11434 || got.Allow.HostPorts[1] != 8881 {
		t.Errorf("stored allow host ports = %v, want [11434 8881]", got.Allow.HostPorts)
	}
	if len(got.Allow.Domains) != 1 || got.Allow.Domains[0] != "api.github.com" {
		t.Errorf("stored allow domains = %v, want [api.github.com]", got.Allow.Domains)
	}
	if len(got.Block.HostPorts) != 1 || got.Block.HostPorts[0] != 9999 {
		t.Errorf("stored block host ports = %v, want [9999]", got.Block.HostPorts)
	}

	// Other metadata survives the policy write.
	if ws, err := GetStoredWorkspace(name); err != nil || ws != "/tmp/ws" {
		t.Errorf("workspace after StoreNetworkPolicy = (%q, %v), want (/tmp/ws, nil)", ws, err)
	}
	if agent, err := GetStoredAgent(name); err != nil || agent != "claude" {
		t.Errorf("agent after StoreNetworkPolicy = (%q, %v), want (claude, nil)", agent, err)
	}

	// Overwriting replaces the previous policy (registry reflects last launch).
	replace := &NetworkPolicy{Allow: &PolicyAllow{HostPorts: []int{7000}}}
	if err := StoreNetworkPolicy(name, replace); err != nil {
		t.Fatalf("StoreNetworkPolicy (overwrite): %v", err)
	}
	got, err = GetStoredNetworkPolicy(name)
	if err != nil {
		t.Fatalf("GetStoredNetworkPolicy (overwrite): %v", err)
	}
	if len(got.Allow.HostPorts) != 1 || got.Allow.HostPorts[0] != 7000 {
		t.Errorf("overwritten policy = %+v, want allow host ports [7000]", got.Allow)
	}
}
