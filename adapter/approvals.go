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
	requester   string
	command     string
	created     time.Time
	reservation *approvalReservation
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
	ErrUnauthorizedApprover = errors.New("only administrators can approve requests")
	ErrRequestInProgress    = errors.New("request already has an approval in progress")
	ErrNoSuchRequest        = errors.New("no pending request with that id (it may have expired or already been approved)")
	ErrSelfApproval         = errors.New("you cannot approve your own request; another administrator must")
	ErrMissingApproveID     = errors.New("give the request id: /repo approve <id>")
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

// approvalReservation binds a policy evaluation to one pending request. It is
// consumed only after successful authorization and policy evaluation.
type approvalReservation struct {
	approvals *Approvals
	id        string
	command   string
}

// Reserve authorizes the Discord principal before changing pending state.
// authorized must come from the trusted interaction role mapping, never command input.
func (a *Approvals) Reserve(id, approver string, authorized bool) (*approvalReservation, error) {
	if !authorized || approver == "" {
		return nil, ErrUnauthorizedApprover
	}
	if id == "" {
		return nil, ErrMissingApproveID
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	req, ok := a.pending[id]
	if !ok || a.now().Sub(req.created) >= pendingTTL {
		delete(a.pending, id)
		return nil, ErrNoSuchRequest
	}
	if req.requester == approver {
		return nil, ErrSelfApproval
	}
	if req.reservation != nil {
		return nil, ErrRequestInProgress
	}
	r := &approvalReservation{approvals: a, id: id, command: req.command}
	req.reservation = r
	a.pending[id] = req
	return r, nil
}

func (r *approvalReservation) cancel() {
	a := r.approvals
	a.mu.Lock()
	defer a.mu.Unlock()
	if req, ok := a.pending[r.id]; ok && req.reservation == r {
		req.reservation = nil
		a.pending[r.id] = req
	}
}

func (r *approvalReservation) commit() error {
	a := r.approvals
	a.mu.Lock()
	defer a.mu.Unlock()
	req, ok := a.pending[r.id]
	if !ok || req.reservation != r {
		return ErrNoSuchRequest
	}
	if a.now().Sub(req.created) >= pendingTTL {
		delete(a.pending, r.id)
		return ErrNoSuchRequest
	}
	delete(a.pending, r.id)
	return nil
}

// Approve is a trusted internal convenience for immediate approval. Discord
// handlers must use Reserve with authenticated roles and commit after policy ALLOW.
func (a *Approvals) Approve(id, approver string) (string, error) {
	r, err := a.Reserve(id, approver, true)
	if err != nil {
		return "", err
	}
	if err := r.commit(); err != nil {
		return "", err
	}
	return r.command, nil
}
