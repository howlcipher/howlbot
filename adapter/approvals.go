package adapter

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"
	"time"
)

// A release that needs approval is stored as a pending request with an id.
// `/repo approve <id>` approves only that stored request, only once, and only
// by someone other than the requester (issue #1). Pending requests live in
// memory: a restart forgets them, which fails closed.

const pendingTTL = 24 * time.Hour

type pendingRequest struct {
	requester string
	command   string
	created   time.Time
}

// Approvals holds the pending requests.
type Approvals struct {
	mu      sync.Mutex
	pending map[string]pendingRequest
	now     func() time.Time
}

func NewApprovals() *Approvals {
	return &Approvals{pending: map[string]pendingRequest{}, now: time.Now}
}

var (
	ErrNoSuchRequest    = errors.New("no pending request with that id (it may have expired or already been approved)")
	ErrSelfApproval     = errors.New("you cannot approve your own request; another administrator must")
	ErrMissingApproveID = errors.New("give the request id: /repo approve <id>")
)

// Request records a pending command and returns its id.
func (a *Approvals) Request(requester, command string) string {
	buf := make([]byte, 4)
	_, _ = rand.Read(buf)
	id := hex.EncodeToString(buf)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.pending[id] = pendingRequest{requester: requester, command: command, created: a.now()}
	return id
}

// Approve consumes the pending request id for approver and returns the
// command to evaluate as approved.
func (a *Approvals) Approve(id, approver string) (string, error) {
	if id == "" {
		return "", ErrMissingApproveID
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	req, ok := a.pending[id]
	if !ok || a.now().Sub(req.created) > pendingTTL {
		delete(a.pending, id)
		return "", ErrNoSuchRequest
	}
	if req.requester == approver {
		return "", ErrSelfApproval
	}
	delete(a.pending, id)
	return req.command, nil
}
