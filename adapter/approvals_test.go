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

func TestReservationRetriesAndConcurrentSingleCommit(t *testing.T) {
	a := NewApprovals()
	id := a.Request("requester", "repo_release")
	if _, err := a.Reserve(id, "outsider", false); !errors.Is(err, ErrUnauthorizedApprover) {
		t.Fatal(err)
	}
	r, err := a.Reserve(id, "admin", true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Reserve(id, "admin2", true); !errors.Is(err, ErrRequestInProgress) {
		t.Fatal(err)
	}
	r.cancel() // Evaluation failed or denied; the same request remains retriable.
	r2, err := a.Reserve(id, "admin2", true)
	if err != nil {
		t.Fatal(err)
	}
	r.cancel() // An old defer must not cancel a newer reservation.
	if _, err := a.Reserve(id, "admin3", true); !errors.Is(err, ErrRequestInProgress) {
		t.Fatal(err)
	}
	results := make(chan error, 20)
	for i := 0; i < 20; i++ {
		go func() { results <- r2.commit() }()
	}
	successes := 0
	for i := 0; i < 20; i++ {
		if <-results == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("successful commits = %d, want 1", successes)
	}
	if _, err := a.Approve(id, "admin"); !errors.Is(err, ErrNoSuchRequest) {
		t.Fatal(err)
	}
}

func TestReservationCannotCommitAfterExpiry(t *testing.T) {
	a := NewApprovals()
	now := time.Now()
	a.now = func() time.Time { return now }
	id := a.Request("requester", "repo_release")
	r, err := a.Reserve(id, "admin", true)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(pendingTTL)
	if err := r.commit(); !errors.Is(err, ErrNoSuchRequest) {
		t.Fatal(err)
	}
}
