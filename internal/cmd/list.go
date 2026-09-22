package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fsan/cloma/internal/sandbox"
	"github.com/spf13/cobra"
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all cloma-managed sandboxes",
	Long: `List all Docker Desktop sandboxes managed by cloma.

Sandboxes managed by cloma have names starting with "cloma-".
The list shows the sandbox name, status, and the recorded workspace path.`,
	RunE: runList,
}

func init() {
	rootCmd.AddCommand(listCmd)
}

// SandboxInfo holds information about a sandbox for display
type SandboxInfo struct {
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Workspace string    `json:"workspace,omitempty"`
	Agent     string    `json:"agent,omitempty"`
	Created   time.Time `json:"created,omitempty"`

	// NetworkPolicy is the effective network policy recorded at the last
	// launch (allowed/blocked host ports and domains). Nil when the
	// sandbox predates network policies.
	NetworkPolicy *sandbox.NetworkPolicy `json:"network_policy,omitempty"`
}

func runList(cmd *cobra.Command, args []string) error {
	// Get all sandboxes
	sandboxes, err := sandbox.List()
	if err != nil {
		return fmt.Errorf("failed to list sandboxes: %w", err)
	}

	// Filter to cloma-managed sandboxes (names starting with "cloma-")
	var clomaSandboxes []SandboxInfo
	for _, sb := range sandboxes {
		if strings.HasPrefix(sb.Name, "cloma-") {
			info := SandboxInfo{
				Name:   sb.Name,
				Status: sb.Status,
			}
			// Look up the workspace from the sandbox registry rather than
			// trying to decode it from the name. Label-based names (created
			// with --name) have no path hash, so name-based decoding does not
			// work for them.
			ws, err := sandbox.GetStoredWorkspace(sb.Name)
			if err == nil {
				info.Workspace = ws
			}
			// Enrich with the agent, creation time, and effective network
			// policy recorded in the registry.
			if agent, err := sandbox.GetStoredAgent(sb.Name); err == nil {
				info.Agent = agent
			}
			if created, err := sandbox.GetCreationTime(sb.Name); err == nil && !created.IsZero() {
				info.Created = created
			}
			if policy, err := sandbox.GetStoredNetworkPolicy(sb.Name); err == nil && policy != nil {
				info.NetworkPolicy = policy
			}
			clomaSandboxes = append(clomaSandboxes, info)
		}
	}

	// Output
	if jsonOutput {
		return outputJSON(clomaSandboxes)
	}

	return outputText(clomaSandboxes)
}

func outputJSON(sandboxes []SandboxInfo) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(sandboxes)
}

func outputText(sandboxes []SandboxInfo) error {
	if len(sandboxes) == 0 {
		fmt.Println("No cloma-managed sandboxes found.")
		return nil
	}

	// Print header
	fmt.Printf("%-50s %-12s %-10s %-20s %-28s %s\n", "NAME", "STATUS", "AGENT", "CREATED", "POLICY", "WORKSPACE")
	fmt.Println(strings.Repeat("-", 140))

	// Print sandboxes
	for _, sb := range sandboxes {
		workspace := sb.Workspace
		if workspace == "" {
			workspace = "<unknown>"
		}
		agent := sb.Agent
		if agent == "" {
			agent = "-"
		}
		created := "-"
		if !sb.Created.IsZero() {
			created = sb.Created.Format("2006-01-02 15:04:05")
		}
		fmt.Printf("%-50s %-12s %-10s %-20s %-28s %s\n", sb.Name, sb.Status, agent, created, formatPolicy(sb.NetworkPolicy), workspace)
	}

	return nil
}

// formatPolicy renders the effective network policy as a compact string:
// allowed host ports as ":<port>", blocked host ports as "!<port>", allowed
// domains as "+<domain>", and blocked domains as "-<domain>". Returns "-"
// when no policy was recorded (sandbox predates network policies).
func formatPolicy(p *sandbox.NetworkPolicy) string {
	if p == nil {
		return "-"
	}
	var parts []string
	if p.Allow != nil {
		for _, port := range p.Allow.HostPorts {
			parts = append(parts, fmt.Sprintf(":%d", port))
		}
		for _, domain := range p.Allow.Domains {
			parts = append(parts, "+"+domain)
		}
	}
	if p.Block != nil {
		for _, port := range p.Block.HostPorts {
			parts = append(parts, fmt.Sprintf("!%d", port))
		}
		for _, domain := range p.Block.Domains {
			parts = append(parts, "-"+domain)
		}
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, " ")
}
