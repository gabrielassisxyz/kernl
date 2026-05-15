package dispatch

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gastownhall/foolery/internal/backend"
)

func nowFrom(deps *ForensicDeps) string {
	if deps != nil && deps.Now != nil {
		return deps.Now()
	}
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func captureBeatSnapshotData(ctx CaptureContext, deps *ForensicDeps) (beat *backend.Beat, leases []backend.Beat, errors []string) {
	if deps == nil || deps.ShowKnot == nil {
		errors = append(errors, "showKnot: dependency not available")
		return nil, nil, errors
	}
	b, err := deps.ShowKnot(ctx.BeatID, ctx.RepoPath)
	if err != nil {
		errors = append(errors, fmt.Sprintf("showKnot: %s", err.Error()))
	} else {
		beat = b
	}

	if deps.ListLeases != nil {
		ls, err := deps.ListLeases(ctx.RepoPath, true)
		if err != nil {
			errors = append(errors, fmt.Sprintf("listLeases: %s", err.Error()))
		} else {
			leases = leasesForBeat(ctx.BeatID, ctx.SessionID, ls)
		}
	}

	return beat, leases, errors
}

func leasesForBeat(beatID, sessionID string, all []backend.Beat) []backend.Beat {
	var matched []backend.Beat
	for _, l := range all {
		nickname, _ := l.Metadata["nickname"].(string)
		if strings.Contains(nickname, beatID) || strings.Contains(nickname, sessionID) {
			matched = append(matched, l)
		}
	}
	if len(matched) == 0 && len(all) <= 10 {
		return all
	}
	return matched
}

func CaptureBeatSnapshot(boundary DispatchForensicBoundary, ctx CaptureContext, deps *ForensicDeps) BeatSnapshot {
	capturedAt := nowFrom(deps)
	base := BeatSnapshot{
		Boundary:      boundary,
		CapturedAt:    capturedAt,
		SessionID:     ctx.SessionID,
		BeatID:        ctx.BeatID,
		AgentInfo:     ctx.AgentInfo,
		LeaseID:       ctx.LeaseID,
		Iteration:     ctx.Iteration,
		ObservedState: ctx.ObservedState,
		ExpectedStep:  ctx.ExpectedStep,
		FooleryPID:    os.Getpid(),
		ChildPID:      ctx.ChildPID,
	}

	beat, leases, captureErrors := captureBeatSnapshotData(ctx, deps)

	snapshot := base
	snapshot.ObservedState = ctx.ObservedState
	if beat != nil && snapshot.ObservedState == "" {
		snapshot.ObservedState = beat.State
	}
	snapshot.Beat = beat
	snapshot.Leases = leases
	if len(captureErrors) > 0 {
		snapshot.CaptureErrors = captureErrors
	}

	writer := deps.Writer
	if writer == nil {
		writer = &fsSnapshotWriter{}
	}

	var snapshotPath string
	pathBytes, err := writer.Write(snapshot)
	if err != nil {
		slog.Error("dispatch forensics write failed", "boundary", boundary, "beat", ctx.BeatID, "error", err)
	} else {
		snapshotPath = pathBytes
	}

	logAudit := deps.LogAudit
	if logAudit == nil {
		logAudit = defaultLogAudit
	}
	logAudit(fmt.Sprintf("beat_snapshot_%s", boundary), map[string]any{
		"message":       fmt.Sprintf("Captured beat snapshot at boundary %s.", boundary),
		"repoPath":      ctx.RepoPath,
		"sessionId":     ctx.SessionID,
		"beatId":        ctx.BeatID,
		"knotsLeaseId":  ctx.LeaseID,
		"iteration":     ctx.Iteration,
		"observedState":  snapshot.ObservedState,
		"expectedStep":  ctx.ExpectedStep,
		"snapshotPath":  snapshotPath,
		"captureErrors": snapshot.CaptureErrors,
	})

	return snapshot
}

func defaultLogAudit(event string, payload map[string]any) {
	slog.Info(event, "payload", payload)
}

func RunPostTurnForensics(pre, post BeatSnapshot, preSnapshotPath, postSnapshotPath string, signals *ClassifierSignals, deps *ForensicDeps) PostTurnForensicResult {
	classification := ClassifyTurnFailure(pre, post, signals)
	if classification == nil {
		return PostTurnForensicResult{Classified: false}
	}

	iteration := pre.Iteration
	if post.Iteration > iteration {
		iteration = post.Iteration
	}

	body := BuildForensicBannerBody(ForensicBannerInput{
		Category:         classification.Category,
		BeatID:           post.BeatID,
		SessionID:        post.SessionID,
		LeaseID:          post.LeaseID,
		Iteration:        iteration,
		PreSnapshotPath:  preSnapshotPath,
		PostSnapshotPath: postSnapshotPath,
		Reasoning:        classification.Reasoning,
	})

	banner := EmitForensicBanner(body)

	if deps != nil && deps.PushBanner != nil {
		deps.PushBanner(banner)
	}

	logAudit := defaultLogAudit
	if deps != nil && deps.LogAudit != nil {
		logAudit = deps.LogAudit
	}
	conflictingLeaseID := ""
	if classification.ConflictingLease != nil {
		conflictingLeaseID = classification.ConflictingLease.ID
	}
	logAudit("dispatch_forensic_classified", map[string]any{
		"message":             fmt.Sprintf("Dispatch forensic classified: %s.", classification.Category),
		"sessionId":           post.SessionID,
		"beatId":              post.BeatID,
		"knotsLeaseId":        post.LeaseID,
		"category":            string(classification.Category),
		"reasoning":           classification.Reasoning,
		"conflictingLeaseId":  conflictingLeaseID,
		"preSnapshotPath":     preSnapshotPath,
		"postSnapshotPath":    postSnapshotPath,
	})

	return PostTurnForensicResult{
		Classified: true,
		BannerBody: body,
	}
}

type fsSnapshotWriter struct{}

func (f *fsSnapshotWriter) Write(snapshot BeatSnapshot) (string, error) {
	logRoot := resolveLogRoot()
	date := snapshot.CapturedAt[:10]
	p := SnapshotPath(SnapshotPathInput{
		LogRoot:    logRoot,
		Date:       date,
		SessionID:  snapshot.SessionID,
		BeatID:     snapshot.BeatID,
		Boundary:   snapshot.Boundary,
		CapturedAt: snapshot.CapturedAt,
	})
	dir := filepath.Dir(p)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("dispatch forensics mkdir: %w", err)
	}
	body, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", fmt.Errorf("dispatch forensics marshal: %w", err)
	}
	if err := os.WriteFile(p, body, 0644); err != nil {
		return "", fmt.Errorf("dispatch forensics write: %w", err)
	}
	return p, nil
}

func resolveLogRoot() string {
	if dir := os.Getenv("FOOLERY_LOG_ROOT"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "foolery", "logs")
	}
	return filepath.Join(home, ".foolery", "logs")
}

var logRootOnce struct {
	sync.Once
	value string
}