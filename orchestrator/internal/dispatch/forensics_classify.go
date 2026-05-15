package dispatch

import (
	"fmt"
	"log/slog"
	"reflect"
	"strings"

	"github.com/gastownhall/foolery/internal/backend"
)

const DispatchForensicsSlug = "_dispatch_forensics"
const DispatchForensicMarker = "FOOLERY DISPATCH FORENSIC"

func safeSegment(value string) string {
	s := strings.ReplaceAll(value, "/", "_")
	s = strings.ReplaceAll(s, "\\", "_")
	s = strings.ReplaceAll(s, ".", "_")
	if len(s) > 0 && s[0] == '.' {
		s = "_" + s[1:]
	}
	return s
}

func SnapshotPath(input SnapshotPathInput) string {
	ts := strings.ReplaceAll(input.CapturedAt, ":", "-")
	ts = strings.ReplaceAll(ts, ".", "-")
	fileName := fmt.Sprintf("%s-%s-%s.json", ts, input.Boundary, safeSegment(input.BeatID))
	return fmt.Sprintf("%s/%s/%s/%s/%s", input.LogRoot, DispatchForensicsSlug, input.Date, safeSegment(input.SessionID), fileName)
}

func stepHistoryOf(beat *backend.Beat) []StepEntry {
	if beat == nil {
		return nil
	}
	raw, ok := beat.Metadata["step_history"]
	if !ok {
		raw, ok = beat.Metadata["stepHistory"]
	}
	if !ok {
		return nil
	}
	return parseStepEntries(raw)
}

func parseStepEntries(raw any) []StepEntry {
	if raw == nil {
		return nil
	}
	v := reflect.ValueOf(raw)
	if v.Kind() != reflect.Slice {
		return nil
	}
	var result []StepEntry
	for i := 0; i < v.Len(); i++ {
		item := v.Index(i).Interface()
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		entry := StepEntry{}
		if id, ok := m["id"].(string); ok {
			entry.ID = id
		}
		if step, ok := m["step"].(string); ok {
			entry.Step = step
		}
		if lid, ok := m["lease_id"].(string); ok {
			entry.LeaseID = lid
		}
		if an, ok := m["agent_name"].(string); ok {
			entry.AgentName = an
		}
		if am, ok := m["agent_model"].(string); ok {
			entry.AgentModel = am
		}
		if av, ok := m["agent_version"].(string); ok {
			entry.AgentVersion = av
		}
		if sa, ok := m["started_at"].(string); ok {
			entry.StartedAt = sa
		}
		if ea, ok := m["ended_at"].(string); ok {
			entry.EndedAt = ea
		}
		if fs, ok := m["from_state"].(string); ok {
			entry.FromState = fs
		}
		if ts, ok := m["to_state"].(string); ok {
			entry.ToState = ts
		}
		result = append(result, entry)
	}
	return result
}

func newStepEntries(pre, post BeatSnapshot) []StepEntry {
	preIDs := make(map[string]bool)
	for _, s := range stepHistoryOf(pre.Beat) {
		if s.ID != "" {
			preIDs[s.ID] = true
		}
	}
	var result []StepEntry
	for _, s := range stepHistoryOf(post.Beat) {
		if s.ID != "" && !preIDs[s.ID] {
			result = append(result, s)
		}
	}
	return result
}

func leaseStateOf(lease *backend.Beat) string {
	if lease == nil {
		return ""
	}
	raw, ok := lease.Metadata["state"]
	if !ok {
		return ""
	}
	if s, ok := raw.(string); ok {
		return s
	}
	return ""
}

func findLeaseByID(leases []backend.Beat, id string) *backend.Beat {
	if leases == nil || id == "" {
		return nil
	}
	for i := range leases {
		if leases[i].ID == id {
			return &leases[i]
		}
	}
	return nil
}

func classifyConcurrentClaim(newSteps []StepEntry, ourLeaseID string, postLeases []backend.Beat) *ForensicClassification {
	for _, s := range newSteps {
		if s.LeaseID != "" && s.LeaseID != ourLeaseID {
			conflicting := findLeaseByID(postLeases, s.LeaseID)
			agentName := s.AgentName
			agentModel := s.AgentModel
			agentVersion := s.AgentVersion
			reasoning := fmt.Sprintf(
				"step_history gained an action step bound to lease %s (agent=%s/%s/%s); our lease was %s. Another agent claimed this beat between our pre_lease and post_turn snapshots.",
				s.LeaseID, agentName, agentModel, agentVersion, ourLeaseID,
			)
			return &ForensicClassification{
				Category:         CategoryConcurrentClaim,
				Reasoning:        reasoning,
				ConflictingLease: conflicting,
			}
		}
	}
	return nil
}

func classifyDoubleClaim(newSteps []StepEntry, ourLeaseID string) *ForensicClassification {
	var ourSteps []StepEntry
	for _, s := range newSteps {
		if s.LeaseID != "" && s.LeaseID == ourLeaseID {
			ourSteps = append(ourSteps, s)
		}
	}
	if len(ourSteps) >= 2 {
		return &ForensicClassification{
			Category:  CategoryDoubleClaim,
			Reasoning: fmt.Sprintf("step_history gained %d new action steps all bound to our lease %s. The dispatched agent appears to have invoked `kno claim` more than once in the same turn.", len(ourSteps), ourLeaseID),
		}
	}
	return nil
}

func classifyHalfTransition(newSteps []StepEntry, ourLeaseID string, signals *ClassifierSignals) *ForensicClassification {
	if signals == nil || !signals.AgentClaimExitedNonZero {
		return nil
	}
	var ourSteps []StepEntry
	for _, s := range newSteps {
		if s.LeaseID != "" && s.LeaseID == ourLeaseID {
			ourSteps = append(ourSteps, s)
		}
	}
	if len(ourSteps) == 0 {
		return nil
	}
	return &ForensicClassification{
		Category:  CategoryHalfTransition,
		Reasoning: fmt.Sprintf("agent's `kno claim` exited non-zero, but step_history contains %d new action step(s) bound to our lease %s. `kno claim` appears to have transitioned the beat state and then errored without rolling back.", len(ourSteps), ourLeaseID),
	}
}

func classifyLeaseTerminated(pre, post BeatSnapshot, ourLeaseID string, signals *ClassifierSignals) *ForensicClassification {
	if signals != nil && signals.FoolerInitiatedLeaseTerminate {
		return nil
	}
	preLease := findLeaseByID(pre.Leases, ourLeaseID)
	postLease := findLeaseByID(post.Leases, ourLeaseID)
	preState := leaseStateOf(preLease)
	postState := leaseStateOf(postLease)
	if preState != "lease_ready" {
		return nil
	}
	if postState != "lease_terminated" {
		return nil
	}
	return &ForensicClassification{
		Category:  CategoryLeaseTerminated,
		Reasoning: fmt.Sprintf("our lease %s moved from lease_ready to lease_terminated between pre_lease and post_turn snapshots, but foolery did not initiate the termination. Likely cause: the dispatched agent ran `kno rollback` (which kno terminates the action step's lease as a side effect).", ourLeaseID),
	}
}

func ClassifyTurnFailure(pre, post BeatSnapshot, signals *ClassifierSignals) *ForensicClassification {
	ourLeaseID := post.LeaseID
	if ourLeaseID == "" {
		ourLeaseID = pre.LeaseID
	}
	newSteps := newStepEntries(pre, post)

	if c := classifyConcurrentClaim(newSteps, ourLeaseID, post.Leases); c != nil {
		return c
	}
	if c := classifyDoubleClaim(newSteps, ourLeaseID); c != nil {
		return c
	}
	if c := classifyHalfTransition(newSteps, ourLeaseID, signals); c != nil {
		return c
	}
	if c := classifyLeaseTerminated(pre, post, ourLeaseID, signals); c != nil {
		return c
	}

	var preState, postState string
	if pre.Beat != nil {
		preState = pre.Beat.State
	}
	if post.Beat != nil {
		postState = post.Beat.State
	}
	if len(newSteps) > 0 || preState != postState {
		return &ForensicClassification{
			Category:  CategoryUnknownStateChange,
			Reasoning: fmt.Sprintf("state changed between snapshots (pre.state=%s -> post.state=%s, new step_history entries: %d) but no named category fits. Read the snapshot files to investigate.", preState, postState, len(newSteps)),
		}
	}

	return nil
}

func BuildForensicBannerBody(input ForensicBannerInput) string {
	heading := fmt.Sprintf("%s: %s on beat %s", DispatchForensicMarker, input.Category, input.BeatID)
	iteration := "?"
	if input.Iteration > 0 {
		iteration = fmt.Sprintf("%d", input.Iteration)
	}
	leaseID := "?"
	if input.LeaseID != "" {
		leaseID = input.LeaseID
	}
	body := fmt.Sprintf(
		"  session      = %s\n  beat         = %s\n  iteration    = %s\n  lease        = %s\n  preSnapshot  = %s\n  postSnapshot = %s\n\n  reasoning:\n    %s",
		input.SessionID, input.BeatID, iteration, leaseID, input.PreSnapshotPath, input.PostSnapshotPath,
		strings.ReplaceAll(input.Reasoning, "\n", "\n    "),
	)
	return heading + "\n" + body
}

type ForensicBannerInput struct {
	Category         ForensicCategory
	BeatID           string
	SessionID        string
	LeaseID          string
	Iteration        int
	PreSnapshotPath  string
	PostSnapshotPath string
	Reasoning        string
}

func EmitForensicBanner(body string) string {
	red := "\x1b[31m"
	reset := "\x1b[0m"
	banner := red + strings.Repeat("=", 72) + reset + "\n" + body + "\n" + red + strings.Repeat("=", 72) + reset
	slog.Error("dispatch forensic banner emitted", "body", body)
	return "\n" + banner + "\n"
}