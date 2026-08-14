package tests

import (
	"bytes"
	"encoding/json"
	"os/exec"
	"testing"
)

type Event struct {
	Command  string `json:"command"`
	UserID   string `json:"user_id"`
	Roles    string `json:"roles"`
	Approved string `json:"approved"`
}

type Decision struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
	Action   string `json:"action"`
}

func runPolicy(t *testing.T, e Event) Decision {
	bin, err := exec.LookPath("howlframe")
	if err != nil {
		t.Skip("howlframe not in PATH, skipping policy tests")
	}

	args := []string{
		"-run",
		"../src/howlbot.howl",
		"command=" + e.Command,
		"user_id=" + e.UserID,
		"roles=" + e.Roles,
		"approved=" + e.Approved,
	}

	cmd := exec.Command(bin, args...)
	var outb, errb bytes.Buffer
	cmd.Stdout = &outb
	cmd.Stderr = &errb

	err = cmd.Run()
	if err != nil {
		t.Fatalf("howlframe failed: %v\nstderr: %s", err, errb.String())
	}

	var d Decision
	err = json.Unmarshal(outb.Bytes(), &d)
	if err != nil {
		t.Fatalf("failed to parse decision JSON: %v\nstdout: %s", err, outb.String())
	}

	return d
}

func TestPolicy_Status(t *testing.T) {
	d := runPolicy(t, Event{Command: "status", Roles: "none"})
	if d.Decision != "ALLOW" {
		t.Errorf("expected ALLOW for status, got %s", d.Decision)
	}
}

func TestPolicy_RepoRelease_Unprivileged(t *testing.T) {
	d := runPolicy(t, Event{Command: "repo_release", Roles: "none"})
	if d.Decision != "DENY" {
		t.Errorf("expected DENY for unprivileged repo_release, got %s", d.Decision)
	}
}

func TestPolicy_RepoRelease_AdminUnapproved(t *testing.T) {
	d := runPolicy(t, Event{Command: "repo_release", Roles: "admin", Approved: "false"})
	if d.Decision != "REQUIRE_APPROVAL" {
		t.Errorf("expected REQUIRE_APPROVAL, got %s", d.Decision)
	}
}

func TestPolicy_RepoRelease_AdminApproved(t *testing.T) {
	d := runPolicy(t, Event{Command: "repo_release", Roles: "admin", Approved: "true"})
	if d.Decision != "ALLOW" {
		t.Errorf("expected ALLOW, got %s", d.Decision)
	}
}

func TestPolicy_ForgedAdminRole(t *testing.T) {
	// The adapter is responsible for building the Roles list from trusted Discord API data.
	// If a user tries to forge it in the command payload itself, the adapter ignores it.
	// We simulate this by passing a payload where the adapter constructed Roles without admin.
	d := runPolicy(t, Event{Command: "repo_release", Roles: "forged_admin_role"})
	if d.Decision != "DENY" {
		t.Errorf("expected DENY, got %s", d.Decision)
	}
}
