package adapter

import (
	"errors"
	"testing"
	"time"
)

func TestApprovalNeedsAPendingRequestAndAnotherApprover(t *testing.T) {
	a := NewApprovals()
	id := a.Request("requester", "repo_release")

	if _, err := a.Approve(id, "requester"); !errors.Is(err, ErrSelfApproval) {
		t.Fatalf("self-approval: %v", err)
	}
	if _, err := a.Approve("nope", "admin2"); !errors.Is(err, ErrNoSuchRequest) {
		t.Fatalf("unknown id: %v", err)
	}
	if _, err := a.Approve("", "admin2"); !errors.Is(err, ErrMissingApproveID) {
		t.Fatalf("missing id: %v", err)
	}
	command, err := a.Approve(id, "admin2")
	if err != nil || command != "repo_release" {
		t.Fatalf("approve = %q, %v", command, err)
	}
	if _, err := a.Approve(id, "admin3"); !errors.Is(err, ErrNoSuchRequest) {
		t.Fatalf("an approval is used once: %v", err)
	}
}

func TestPendingRequestsExpire(t *testing.T) {
	a := NewApprovals()
	start := time.Now()
	a.now = func() time.Time { return start }
	id := a.Request("requester", "repo_release")
	a.now = func() time.Time { return start.Add(pendingTTL + time.Minute) }
	if _, err := a.Approve(id, "admin2"); !errors.Is(err, ErrNoSuchRequest) {
		t.Fatalf("expired request: %v", err)
	}
}
