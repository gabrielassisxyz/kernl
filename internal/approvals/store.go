package approvals

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Action is the canonical answer to a pending request.
//
// Both client vocabularies map onto these: the orchestrator gate's
// approve/reject and a session's accept/always_approve/decline. Storing the
// canonical form rather than the word the caller typed is what lets the CLI,
// the GUI and the API all resolve the same request without one of them
// recording an answer the bridge cannot act on.
type Action string

const (
	ActionAllow       Action = "allow"
	ActionAllowAlways Action = "allow_always"
	ActionDeny        Action = "deny"
	// ActionExpire is a denial nobody typed: the request outlived its
	// deadline. It is distinct from ActionDeny so a listing can tell "a human
	// refused this" from "no human ever saw it".
	ActionExpire Action = "expire"
)

// Status literals a request can be listed under. They match the vocabulary
// internal/terminal already uses for the same concept.
const (
	StatusPending        = "pending"
	StatusApproved       = "approved"
	StatusAlwaysApproved = "always_approved"
	StatusRejected       = "rejected"
	StatusExpired        = "expired"
)

// Decision is the answer file: the only thing a resolver ever writes.
type Decision struct {
	Action    Action `json:"action"`
	Reason    string `json:"reason,omitempty"`
	DecidedAt string `json:"decidedAt"`
}

// ErrNotFound is returned for an approval id that has no request file. It is a
// distinct error because "you named something that does not exist" is a
// caller's mistake (404) while a read failure is the store's (500).
var ErrNotFound = errors.New("approval not found")

// Store is the file-backed home of pending approvals.
//
// Two files per approval, with exactly one writer each: `<id>.request.json` is
// written once by the bridge that raised the gate, `<id>.decision.json` once by
// whoever answers it. That split is what makes the store safe across processes
// without a lock - no two writers ever touch the same file, so a reader can
// never observe a torn record. It is also why the store is the transport: the
// blocked agent's bridge, `kernl approval resolve`, the REST API and the GUI
// are four different processes, and the directory is the only thing they all
// reach without one of them having to know another's port.
type Store struct {
	dir string
}

func NewStore(dir string) (*Store, error) {
	if dir == "" {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: approval store built with an empty directory - Fix: pass <state dir>/approvals (app.DefaultStateDir() outside tests)")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: creating approval store %s: %w", dir, err)
	}
	return &Store{dir: dir}, nil
}

func (s *Store) Dir() string { return s.dir }

// NewID mints an approval id that sorts by creation time, so a directory
// listing is already in the order a human wants to answer them.
func NewID() string {
	var b [4]byte
	// A failed read would make ids collide, and a colliding id silently
	// answers the wrong gate - the one mistake this whole subsystem exists to
	// prevent. Fall back to nothing: panic is not available to a library, so
	// widen the timestamp instead.
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("apr-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("apr-%d-%s", time.Now().UnixMilli(), hex.EncodeToString(b[:]))
}

// validID rejects anything that could escape the store directory. Ids reach
// this package from the CLI and from HTTP path segments, so a caller-supplied
// "../../etc/passwd" must not become a file path.
func validID(id string) error {
	if id == "" {
		return fmt.Errorf("KERNL DISPATCH FAILURE: empty approval id")
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return fmt.Errorf("KERNL DISPATCH FAILURE: approval id %q contains %q - ids are letters, digits, - and _ only", id, string(r))
		}
	}
	return nil
}

func (s *Store) requestPath(id string) string  { return filepath.Join(s.dir, id+".request.json") }
func (s *Store) decisionPath(id string) string { return filepath.Join(s.dir, id+".decision.json") }

// Create parks a new pending request. It fills in the id, timestamps and
// deadline the caller did not set, so every record in the store carries them.
func (s *Store) Create(req *ApprovalRequest, ttl time.Duration) (*ApprovalRequest, error) {
	if req == nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: Create called with a nil approval request")
	}
	if req.ID == "" {
		req.ID = NewID()
	}
	if err := validID(req.ID); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if req.CreatedAt == "" {
		req.CreatedAt = now.Format(time.RFC3339)
	}
	req.UpdatedAt = req.CreatedAt
	if req.ExpiresAt == "" && ttl > 0 {
		req.ExpiresAt = now.Add(ttl).Format(time.RFC3339)
	}
	req.Status = StatusPending
	req.Actionable = true
	if req.NotificationKey == "" {
		req.NotificationKey = BuildApprovalLogicalKey(req)
	}

	if err := writeJSONAtomic(s.requestPath(req.ID), req); err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: writing approval request %s: %w", req.ID, err)
	}
	return req, nil
}

// Get returns one request with its decision already folded in.
func (s *Store) Get(id string) (*ApprovalRequest, error) {
	if err := validID(id); err != nil {
		return nil, err
	}
	var req ApprovalRequest
	if err := readJSON(s.requestPath(id), &req); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, id)
		}
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: reading approval %s: %w", id, err)
	}
	if err := s.applyDecision(&req); err != nil {
		return nil, err
	}
	return &req, nil
}

// List returns every request the filter admits, oldest first.
func (s *Store) List(filter ApprovalFilter) ([]*ApprovalRequest, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []*ApprovalRequest{}, nil
		}
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: listing approval store %s: %w", s.dir, err)
	}

	out := make([]*ApprovalRequest, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".request.json") {
			continue
		}
		req, err := s.Get(strings.TrimSuffix(name, ".request.json"))
		if err != nil {
			// One unreadable record must not blank the whole listing: a
			// human answering gates needs to see the other nine.
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		if !matchesFilter(req, filter) {
			continue
		}
		out = append(out, req)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].CreatedAt < out[j].CreatedAt })
	return out, nil
}

// Decide records the answer to a pending request.
//
// An unknown id is an error rather than a silent no-op: the scaffolding this
// replaced returned nil for one, which is how `kernl approval resolve apr-999`
// used to print "Resolved" for an approval that never existed.
func (s *Store) Decide(id string, action Action, reason string) (*ApprovalRequest, error) {
	req, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	switch action {
	case ActionAllow, ActionAllowAlways, ActionDeny, ActionExpire:
	default:
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: unknown approval action %q - valid: allow, allow_always, deny, expire", action)
	}
	// The "already answered" check reads the decision file rather than the
	// derived status, because a request past its deadline derives as expired
	// and recording ActionExpire is precisely how that fact gets written down.
	// Testing the derived status here made the bridge unable to close out its
	// own timeout.
	if _, answered, err := s.Decision(id); err != nil {
		return nil, err
	} else if answered {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: approval %s is already %s and cannot be answered again - Run: kernl approval list", id, req.Status)
	}
	if req.Status == StatusExpired && action != ActionExpire {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: approval %s expired and the agent that raised it is no longer waiting - Run: kernl approval list", id)
	}

	decision := Decision{Action: action, Reason: reason, DecidedAt: time.Now().UTC().Format(time.RFC3339)}
	if err := writeJSONAtomic(s.decisionPath(id), decision); err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: writing approval decision %s: %w", id, err)
	}
	applyDecisionTo(req, &decision)
	return req, nil
}

// Decision reads the answer to a request, if one has been given.
func (s *Store) Decision(id string) (*Decision, bool, error) {
	if err := validID(id); err != nil {
		return nil, false, err
	}
	var d Decision
	if err := readJSON(s.decisionPath(id), &d); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("KERNL DISPATCH FAILURE: reading approval decision %s: %w", id, err)
	}
	return &d, true, nil
}

// RememberedAllow reports whether this session already answered a request for
// this tool with always_approve. It is what makes "always" mean anything: the
// bridge consults it before parking a new gate, so the same prompt shape does
// not come back a second time.
func (s *Store) RememberedAllow(sessionID, toolName string) (bool, error) {
	if sessionID == "" || toolName == "" {
		return false, nil
	}
	all, err := s.List(ApprovalFilter{Status: StatusAlwaysApproved})
	if err != nil {
		return false, err
	}
	for _, req := range all {
		if req.SessionID == sessionID && req.ToolName == toolName {
			return true, nil
		}
	}
	return false, nil
}

func (s *Store) applyDecision(req *ApprovalRequest) error {
	decision, ok, err := s.Decision(req.ID)
	if err != nil {
		return err
	}
	if ok {
		applyDecisionTo(req, decision)
		return nil
	}
	// No answer yet. A deadline that has passed makes the request
	// unanswerable even though no decision file was ever written - the
	// bridge that would have written one may have been killed with its run.
	if req.ExpiresAt != "" {
		deadline, parseErr := time.Parse(time.RFC3339, req.ExpiresAt)
		if parseErr == nil && time.Now().After(deadline) {
			req.Status = StatusExpired
			req.Actionable = false
			req.UpdatedAt = req.ExpiresAt
			req.Reason = "no answer before the approval deadline"
			return nil
		}
	}
	req.Status = StatusPending
	req.Actionable = true
	return nil
}

func applyDecisionTo(req *ApprovalRequest, d *Decision) {
	switch d.Action {
	case ActionAllow:
		req.Status = StatusApproved
	case ActionAllowAlways:
		req.Status = StatusAlwaysApproved
	case ActionExpire:
		req.Status = StatusExpired
	default:
		req.Status = StatusRejected
	}
	req.Actionable = false
	req.DecidedAt = d.DecidedAt
	req.UpdatedAt = d.DecidedAt
	req.Reason = d.Reason
}

func matchesFilter(req *ApprovalRequest, f ApprovalFilter) bool {
	if f.ActiveOnly && req.Status != StatusPending {
		return false
	}
	if f.RepoPath != "" && req.RepoPath != f.RepoPath {
		return false
	}
	if f.Status != "" && req.Status != f.Status {
		return false
	}
	// RFC3339 in a fixed zone sorts lexicographically, which is why the store
	// writes every timestamp in UTC.
	if f.UpdatedSince != "" && req.UpdatedAt < f.UpdatedSince {
		return false
	}
	return true
}

func readJSON(path string, dst any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

// writeJSONAtomic writes through a temp file in the same directory, so a
// reader in another process sees either the whole record or no record. A
// half-written approval would be read as a malformed gate on the one path
// where guessing is unacceptable.
func writeJSONAtomic(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}
