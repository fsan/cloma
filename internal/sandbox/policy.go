package sandbox

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// NetworkPolicy is the parsed form of a --network-policy YAML file. It
// describes which host ports and external domains the sandbox may reach,
// and which should be explicitly blocked.
//
// The sandbox network is default-deny: a host port is reachable only after
// cloma registers it with the Docker sandbox proxy
// (`docker sandbox network proxy <name> --allow-host localhost:<port>`).
// Blocking a host port therefore means "do not allow it" — in particular it
// can be used to revoke cloma's automatic allow of the Ollama port. External
// domains, in contrast, are reachable through the sandbox microVM's outbound
// egress unless denied, so blocking them is an active operation performed
// with the in-sandbox `sbx policy` CLI.
type NetworkPolicy struct {
	// Allow lists what the sandbox may reach in addition to the default.
	Allow *PolicyAllow `yaml:"allow" json:"allow,omitempty"`

	// Block lists what the sandbox may not reach. Block entries win over
	// allow entries (a conflict is rejected at load time, and blocking the
	// Ollama port revokes cloma's automatic allow).
	Block *PolicyBlock `yaml:"block" json:"block,omitempty"`
}

// PolicyAllow is the `allow` section of a network policy file.
type PolicyAllow struct {
	// HostPorts are host ports reachable from the sandbox as
	// host.docker.internal:<port> (translated to localhost:<port> by the
	// sandbox proxy).
	HostPorts []int `yaml:"host_ports" json:"host_ports,omitempty"`

	// Domains are external hostnames the sandbox may reach over the
	// internet, e.g. "api.github.com".
	Domains []string `yaml:"domains" json:"domains,omitempty"`
}

// PolicyBlock is the `block` section of a network policy file.
type PolicyBlock struct {
	// HostPorts are host ports that must NOT be reachable from the
	// sandbox. Because host access is default-deny, this only revokes
	// ports cloma would otherwise allow (the Ollama port by default).
	HostPorts []int `yaml:"host_ports" json:"host_ports,omitempty"`

	// Domains are external hostnames the sandbox may not reach.
	Domains []string `yaml:"domains" json:"domains,omitempty"`
}

// LoadNetworkPolicy reads and validates a network policy YAML file. It
// returns an error for malformed YAML, unknown keys, out-of-range ports,
// host-style entries in `domains`, and entries present in both the allow
// and block sections — failing fast before any sandbox is created.
func LoadNetworkPolicy(path string) (*NetworkPolicy, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read network policy file: %w", err)
	}

	var policy NetworkPolicy
	// Strict decoding: a typo like `host_port:` fails loudly instead of
	// silently allowing nothing.
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&policy); err != nil {
		if errors.Is(err, io.EOF) {
			// An empty file is a valid (if useless) policy.
			return &policy, nil
		}
		return nil, fmt.Errorf("failed to parse network policy file: %w", err)
	}

	if err := policy.validate(); err != nil {
		return nil, fmt.Errorf("invalid network policy: %w", err)
	}
	return &policy, nil
}

// validate checks a parsed policy for out-of-range ports, malformed or
// host-style domains, and allow/block conflicts.
func (p *NetworkPolicy) validate() error {
	var allowPorts, blockPorts []int
	var allowDomains, blockDomains []string
	if p != nil {
		if p.Allow != nil {
			allowPorts = p.Allow.HostPorts
			allowDomains = p.Allow.Domains
		}
		if p.Block != nil {
			blockPorts = p.Block.HostPorts
			blockDomains = p.Block.Domains
		}
	}

	for _, entry := range []struct {
		section string
		ports   []int
	}{{"allow", allowPorts}, {"block", blockPorts}} {
		for _, port := range entry.ports {
			if port < 1 || port > 65535 {
				return fmt.Errorf("%s.host_ports: port %d out of range (1-65535)", entry.section, port)
			}
		}
	}

	for _, entry := range []struct {
		section string
		domains []string
	}{{"allow", allowDomains}, {"block", blockDomains}} {
		for _, domain := range entry.domains {
			if strings.ContainsAny(domain, " /:") || strings.Contains(domain, "://") {
				return fmt.Errorf("%s.domains: %q is not a bare hostname (no scheme, path or spaces)", entry.section, domain)
			}
			if domain == "localhost" || domain == "host.docker.internal" || domain == "127.0.0.1" {
				return fmt.Errorf("%s.domains: %q refers to the host; list its port under %s.host_ports instead",
					entry.section, domain, entry.section)
			}
		}
	}

	// Allow and block must not overlap; a conflicting entry is almost
	// certainly a mistake, so reject it instead of silently letting
	// block win.
	for _, port := range allowPorts {
		if containsPort(blockPorts, port) {
			return fmt.Errorf("host port %d appears in both allow.host_ports and block.host_ports", port)
		}
	}
	for _, domain := range allowDomains {
		if domainSet(blockDomains)[domain] {
			return fmt.Errorf("domain %q appears in both allow.domains and block.domains", domain)
		}
	}

	return nil
}

func portsOf(a *PolicyAllow) []int {
	if a == nil {
		return nil
	}
	return a.HostPorts
}

func domainsOf(a *PolicyAllow) []string {
	if a == nil {
		return nil
	}
	return a.Domains
}

func blockPortsOf(b *PolicyBlock) []int {
	if b == nil {
		return nil
	}
	return b.HostPorts
}

func blockDomainsOf(b *PolicyBlock) []string {
	if b == nil {
		return nil
	}
	return b.Domains
}

// Summary renders the policy as a single human-readable line, e.g.
// "allow host ports [8881], domains [api.github.com]; block host ports [11434]".
// Empty sections are omitted.
func (p *NetworkPolicy) Summary() string {
	if p == nil {
		return "allow host ports [11434] (default Ollama port)"
	}
	var parts []string

	if ports := portsOf(p.Allow); len(ports) > 0 {
		parts = append(parts, fmt.Sprintf("allow host ports %v", ports))
	}
	if domains := domainsOf(p.Allow); len(domains) > 0 {
		parts = append(parts, fmt.Sprintf("allow domains %v", domains))
	}
	if ports := blockPortsOf(p.Block); len(ports) > 0 {
		parts = append(parts, fmt.Sprintf("block host ports %v", ports))
	}
	if domains := blockDomainsOf(p.Block); len(domains) > 0 {
		parts = append(parts, fmt.Sprintf("block domains %v", domains))
	}
	if len(parts) == 0 {
		return "no entries (default-deny: only the Ollama host port is allowed)"
	}
	return strings.Join(parts, "; ")
}

// Effective returns the policy as it applies to a sandbox launched with
// the given Ollama port: cloma's default Ollama allow merged with the
// file's allow entries, with block entries removed from the allow set
// (block wins). Block entries are preserved in full so callers (and the UI)
// can still show what is denied. A nil policy yields the default: only the
// Ollama port allowed.
//
// The returned policy is what gets applied to the sandbox and recorded in
// the registry, so `cloma list` and the menu bar app can show what is
// actually reachable.
func (p *NetworkPolicy) Effective(ollamaPort int) *NetworkPolicy {
	if p == nil {
		return &NetworkPolicy{Allow: &PolicyAllow{HostPorts: []int{ollamaPort}}}
	}

	blockedPorts := portSet(blockPortsOf(p.Block))
	blockedDomains := domainSet(blockDomainsOf(p.Block))

	effective := &NetworkPolicy{Allow: &PolicyAllow{}, Block: p.Block}
	if !blockedPorts[ollamaPort] {
		effective.Allow.HostPorts = append(effective.Allow.HostPorts, ollamaPort)
	}
	for _, port := range portsOf(p.Allow) {
		// validate() rejects allow/block conflicts, so a blocked port
		// cannot appear here; dedupe guards against a port equal to the
		// Ollama port being listed twice.
		if !blockedPorts[port] && !containsPort(effective.Allow.HostPorts, port) {
			effective.Allow.HostPorts = append(effective.Allow.HostPorts, port)
		}
	}
	for _, domain := range domainsOf(p.Allow) {
		if !blockedDomains[domain] && !containsDomain(effective.Allow.Domains, domain) {
			effective.Allow.Domains = append(effective.Allow.Domains, domain)
		}
	}
	return effective
}

// ApplyNetworkPolicy configures the sandbox's network access to match the
// policy. ollamaPort is the port cloma allows by default so the agent can
// reach its model backend; when the policy blocks it, the allow is revoked
// and a loud warning is produced. All entries that could not be applied are
// returned as warnings rather than aborting the launch, mirroring how
// proxy configuration failures are handled elsewhere in cloma.
func (c *SandboxClient) ApplyNetworkPolicy(sandboxName string, policy *NetworkPolicy, ollamaPort int) []string {
	var warnings []string

	// Blocking the Ollama port revokes cloma's automatic allow; warn
	// loudly because the agent will lose its model backend.
	if portSet(blockPortsOf(policyBlockOf(policy)))[ollamaPort] {
		warnings = append(warnings, fmt.Sprintf(
			"host port %d (Ollama) is blocked by the policy; the agent will NOT be able to reach its model backend",
			ollamaPort))
	}

	// Host ports: default-deny, so "blocked" ports are simply not
	// registered with the proxy. Blocked ports that are not otherwise
	// allowed need no action at all. Effective() already merged the
	// default Ollama allow with the allow entries, minus blocks.
	for _, port := range policy.Effective(ollamaPort).Allow.HostPorts {
		if err := c.allowHostPort(sandboxName, port); err != nil {
			warnings = append(warnings, fmt.Sprintf("Could not allow host port %d: %v", port, err))
		}
	}

	// External domains: allowed entries (block-filtered by Effective)
	// are granted; blocked entries are denied actively.
	for _, domain := range policy.Effective(ollamaPort).Allow.Domains {
		if err := c.allowDomain(sandboxName, domain); err != nil {
			warnings = append(warnings, fmt.Sprintf("Could not allow domain %s: %v", domain, err))
		}
	}
	for _, domain := range blockDomainsOf(policyBlockOf(policy)) {
		if err := c.blockDomain(sandboxName, domain); err != nil {
			warnings = append(warnings, fmt.Sprintf("Could not block domain %s: %v", domain, err))
		}
	}

	return warnings
}

// policyBlockOf returns the block section of a policy, nil-safe.
func policyBlockOf(p *NetworkPolicy) *PolicyBlock {
	if p == nil {
		return nil
	}
	return p.Block
}

// allowDomain grants the sandbox outbound access to an external domain.
// Two mechanisms are tried in order because the Docker sandbox CLI's
// support for domains (as opposed to host ports) varies by version:
// first the host-side proxy registration used for host ports, then the
// in-sandbox `sbx policy allow` CLI invoked from the host via exec.
func (c *SandboxClient) allowDomain(sandboxName, domain string) error {
	if err := c.allowHost(sandboxName, domain); err == nil {
		return nil
	}
	if _, err := Exec(sandboxName, "sbx", "policy", "allow", domain); err == nil {
		return nil
	}
	return fmt.Errorf("neither `docker sandbox network proxy --allow-host` nor `sbx policy allow` succeeded")
}

// blockDomain denies the sandbox outbound access to an external domain.
// Domains are reachable through the microVM's egress unless denied, and
// the only handle on that policy is the in-sandbox `sbx policy` CLI, so
// both plausible deny verbs are tried before giving up.
func (c *SandboxClient) blockDomain(sandboxName, domain string) error {
	for _, verb := range []string{"deny", "block"} {
		if _, err := Exec(sandboxName, "sbx", "policy", verb, domain); err == nil {
			return nil
		}
	}
	return fmt.Errorf("`sbx policy deny/block` not supported by the installed sandbox CLI")
}

func portSet(ports []int) map[int]bool {
	set := make(map[int]bool, len(ports))
	for _, port := range ports {
		set[port] = true
	}
	return set
}

func domainSet(domains []string) map[string]bool {
	set := make(map[string]bool, len(domains))
	for _, domain := range domains {
		set[domain] = true
	}
	return set
}

func containsPort(ports []int, port int) bool {
	for _, p := range ports {
		if p == port {
			return true
		}
	}
	return false
}

func containsDomain(domains []string, domain string) bool {
	for _, d := range domains {
		if d == domain {
			return true
		}
	}
	return false
}
