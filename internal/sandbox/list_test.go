package sandbox

import (
	"strings"
	"testing"
)

func TestParseSandboxList(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    []string
		wantErr string
	}{
		{
			name:   "plain vms key",
			output: `{"vms":[{"name":"cloma-a","status":"running"},{"name":"cloma-b","status":"stopped"}]}`,
			want:   []string{"cloma-a", "cloma-b"},
		},
		{
			name:   "plain sandboxes key",
			output: `{"sandboxes":[{"name":"cloma-c","status":"running"}]}`,
			want:   []string{"cloma-c"},
		},
		{
			name: "daemon bootstrap banner before json",
			output: `Starting sandboxd daemon...
Daemon started (PID: 55276, socket: /tmp/docker_sandboxd.sock)
Logs: /Users/user/.docker/sandboxd/daemon.log
{
  "sandboxes": [
    {"name": "cloma-proof", "status": "running"}
  ]
}`,
			want: []string{"cloma-proof"},
		},
		{
			name:   "banner containing braces",
			output: "Logs: C:\\Users\\{weird}\\sandboxd\\daemon.log\n{\"vms\":[{\"name\":\"cloma-x\",\"status\":\"running\"}]}",
			want:   []string{"cloma-x"},
		},
		{
			name:   "empty list",
			output: `{"vms":[]}`,
			want:   nil,
		},
		{
			name:    "no json at all",
			output:  "Starting sandboxd daemon...\nDaemon failed to start.\n",
			wantErr: "no JSON document found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseSandboxList([]byte(tt.output))
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %d sandboxes, want %d", len(got), len(tt.want))
			}
			for i, name := range tt.want {
				if got[i].Name != name {
					t.Errorf("sandbox %d: got name %q, want %q", i, got[i].Name, name)
				}
			}
		})
	}
}
