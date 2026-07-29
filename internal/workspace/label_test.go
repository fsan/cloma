package workspace

import (
	"strings"
	"testing"
)

func TestResolveSandboxName(t *testing.T) {
	tests := []struct {
		name    string
		label   string
		maxName int
		want    string
		wantErr bool
	}{
		{
			name:    "simple label gets cloma prefix",
			label:   "instance1",
			maxName: 64,
			want:    "cloma-instance1",
		},
		{
			name:    "uppercase and special chars slugified",
			label:   "My Instance #2",
			maxName: 64,
			want:    "cloma-my-instance-2",
		},
		{
			name:    "already prefixed name is idempotent",
			label:   "cloma-instance1",
			maxName: 64,
			want:    "cloma-instance1",
		},
		{
			name:    "full name from list is preserved",
			label:   "cloma-myproject-a1b2c3d4",
			maxName: 64,
			want:    "cloma-myproject-a1b2c3d4",
		},
		{
			name:    "label with only special chars errors",
			label:   "!!!###",
			maxName: 64,
			wantErr: true,
		},
		{
			name:    "empty label errors",
			label:   "",
			maxName: 64,
			wantErr: true,
		},
		{
			name:    "truncation fits within budget",
			label:   "this-is-a-very-long-instance-label-that-exceeds-the-budget",
			maxName: 20,
			want:    "cloma-this-is-a-very", // 20 chars, no trailing hyphen
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSandboxNameWithMaxLen(tt.label, tt.maxName)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("resolveSandboxNameWithMaxLen(%q, %d) err = nil, want error", tt.label, tt.maxName)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveSandboxNameWithMaxLen(%q, %d) unexpected error: %v", tt.label, tt.maxName, err)
			}
			if got != tt.want {
				t.Errorf("resolveSandboxNameWithMaxLen(%q, %d) = %q, want %q", tt.label, tt.maxName, got, tt.want)
			}
			if len(got) > tt.maxName {
				t.Errorf("result %q (len %d) exceeds budget %d", got, len(got), tt.maxName)
			}
		})
	}
}

func TestResolveSandboxNameAlwaysPrefixed(t *testing.T) {
	// Every valid result must carry the cloma- prefix so `cloma list` finds it.
	for _, label := range []string{"foo", "Foo Bar", "cloma-already"} {
		got, err := ResolveSandboxName(label)
		if err != nil {
			t.Fatalf("ResolveSandboxName(%q) unexpected error: %v", label, err)
		}
		if !strings.HasPrefix(got, "cloma-") {
			t.Errorf("ResolveSandboxName(%q) = %q, want cloma- prefix", label, got)
		}
	}
}

func TestResolveSandboxNameNoCollision(t *testing.T) {
	// Two distinct labels must yield distinct sandbox names; the same label
	// must yield the same name. This is what lets users run several instances
	// from one folder without colliding.
	a, _ := ResolveSandboxName("instance1")
	b, _ := ResolveSandboxName("instance2")
	if a == b {
		t.Errorf("distinct labels collided: %q == %q", a, b)
	}
	a2, _ := ResolveSandboxName("instance1")
	if a != a2 {
		t.Errorf("same label not stable: %q != %q", a, a2)
	}
}
