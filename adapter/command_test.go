package adapter

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"testing"
)

func TestPolicyCommandMatchesThePolicyNames(t *testing.T) {
	cases := []struct{ name, sub, command, approved string }{
		{"howl", "status", "status", "false"},
		{"howl", "policy", "policy", "false"},
		{"howl", "history", "history", "false"},
		{"repo", "status", "repo_status", "false"},
		{"repo", "release", "repo_release", "false"},
		{"repo", "approve", "repo_release", "true"},
	}
	for _, c := range cases {
		command, approved := PolicyCommand(c.name, c.sub)
		if command != c.command || approved != c.approved {
			t.Errorf("PolicyCommand(%q, %q) = %q, %q; want %q, %q", c.name, c.sub, command, approved, c.command, c.approved)
		}
	}
}

// Every registered slash command, mapped as the adapter maps it, must reach a
// branch of the HowlFrame policy rather than its "unknown command" default.
func TestEverySlashCommandReachesAPolicyBranch(t *testing.T) {
	bin, err := exec.LookPath("howlframe")
	if err != nil {
		t.Skip("howlframe not in PATH")
	}
	for _, top := range commands {
		for _, opt := range top.Options {
			command, approved := PolicyCommand(top.Name, opt.Name)
			out, err := exec.Command(bin, "-run", "../src/howlbot.howl", "command="+command,
				"user_id=u1", "roles=admin", "approved="+approved).Output()
			if err != nil {
				t.Fatalf("/%s %s: howlframe: %v", top.Name, opt.Name, err)
			}
			var d struct{ Decision, Reason string }
			if err := json.Unmarshal(bytes.TrimSpace(out), &d); err != nil {
				t.Fatalf("/%s %s: not JSON: %s", top.Name, opt.Name, out)
			}
			if d.Reason == "unknown command" {
				t.Errorf("/%s %s (sent as %q) is unknown to the policy", top.Name, opt.Name, command)
			}
		}
	}
}
