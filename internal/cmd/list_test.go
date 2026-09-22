package cmd

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/fsan/cloma/internal/sandbox"
)

func TestFormatPolicy(t *testing.T) {
	tests := []struct {
		name string
		p    *sandbox.NetworkPolicy
		want string
	}{
		{
			name: "nil policy (sandbox predates policies)",
			p:    nil,
			want: "-",
		},
		{
			name: "empty policy",
			p:    &sandbox.NetworkPolicy{},
			want: "none",
		},
		{
			name: "full policy",
			p: &sandbox.NetworkPolicy{
				Allow: &sandbox.PolicyAllow{
					HostPorts: []int{11434, 8881},
					Domains:   []string{"api.github.com"},
				},
				Block: &sandbox.PolicyBlock{
					HostPorts: []int{9999},
					Domains:   []string{"example.com"},
				},
			},
			want: ":11434 :8881 +api.github.com !9999 -example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatPolicy(tt.p); got != tt.want {
				t.Fatalf("formatPolicy() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSandboxInfoNetworkPolicyJSON(t *testing.T) {
	// The menu bar app consumes `cloma list --json`; the network policy
	// must round-trip under the expected keys.
	info := SandboxInfo{
		Name:   "cloma-demo",
		Status: "running",
		NetworkPolicy: &sandbox.NetworkPolicy{
			Allow: &sandbox.PolicyAllow{
				HostPorts: []int{11434, 8881},
				Domains:   []string{"api.github.com"},
			},
			Block: &sandbox.PolicyBlock{
				HostPorts: []int{9999},
				Domains:   []string{"example.com"},
			},
		},
	}

	data, err := json.Marshal(info)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	got := string(data)

	for _, want := range []string{
		`"network_policy"`,
		`"allow"`,
		`"host_ports":[11434,8881]`,
		`"domains":["api.github.com"]`,
		`"block"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("JSON = %s, missing %s", got, want)
		}
	}

	// A nil policy must be omitted entirely so the renderer can
	// distinguish "no policy recorded" from "nothing allowed".
	data, err = json.Marshal(SandboxInfo{Name: "cloma-old", Status: "stopped"})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if strings.Contains(string(data), "network_policy") {
		t.Errorf("JSON = %s, network_policy should be omitted when nil", data)
	}
}
