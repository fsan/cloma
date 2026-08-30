package sandbox

import "testing"

func TestNormalizeAgent(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", AgentClaude},
		{"claude", AgentClaude},
		{"CLAUDE", AgentClaude}, // unknown value falls back to default
		{"grok", AgentGrok},
		{"kimi", AgentKimi},
		{"openclaw", AgentOpenClaw},
		{"junie", AgentJunie},
		{"pi", AgentPi},
		{"unknown", AgentClaude},
	}
	for _, c := range cases {
		if got := NormalizeAgent(c.in); got != c.want {
			t.Errorf("NormalizeAgent(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestWithAgent(t *testing.T) {
	for _, agent := range []string{AgentClaude, AgentGrok, AgentKimi, AgentOpenClaw, AgentJunie, AgentPi} {
		c := NewClient(WithAgent(agent))
		if c.Agent != agent {
			t.Errorf("WithAgent(%q): c.Agent = %q, want %q", agent, c.Agent, agent)
		}
	}

	// Unknown agents normalize back to the default (Claude Code).
	c := NewClient(WithAgent("bogus"))
	if c.Agent != AgentClaude {
		t.Errorf("WithAgent(%q): c.Agent = %q, want %q", "bogus", c.Agent, AgentClaude)
	}
}
