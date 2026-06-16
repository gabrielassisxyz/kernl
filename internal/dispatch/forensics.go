package dispatch

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

func nowFrom(deps *ForensicDeps) string {
	if deps != nil && deps.Now != nil {
		return deps.Now()
	}
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func captureBeadSnapshotData(ctx CaptureContext, deps *ForensicDeps) (bead *backend.Bead, leases []backend.Bead, errors []string) {
	if deps == nil || deps.ShowKnot == nil {
		errors = append(errors, "showKnot: dependency not available")
		return nil, nil, errors
	}
	b, err := deps.ShowKnot(ctx.BeadID, ctx.RepoPath)
	if err != nil {
		errors = append(errors, fmt.Sprintf("showKnot: %s", err.Error()))
	} else {
		bead = b
	}

	if deps.ListLeases != nil {
		ls, err := deps.ListLeases(ctx.RepoPath, true)
		if err != nil {
			errors = append(errors, fmt.Sprintf("listLeases: %s", err.Error()))
		} else {
			leases = leasesForBead(ctx.BeadID, ctx.SessionID, ls)
		}
	}

	return bead, leases, errors
}

func leasesForBead(beadID, sessionID string, all []backend.Bead) []backend.Bead {
	var matched []backend.Bead
	for _, l := range all {
		nickname, _ := l.Metadata["nickname"].(string)
		if strings.Contains(nickname, beadID) || strings.Contains(nickname, sessionID) {
			matched = append(matched, l)
		}
	}
	if len(matched) == 0 && len(all) <= 10 {
		return all
	}
	return matched
}

func CaptureBeadSnapshot(boundary DispatchForensicBoundary, ctx CaptureContext, deps *ForensicDeps) BeadSnapshot {
	capturedAt := nowFrom(deps)
	base := BeadSnapshot{
		Boundary:      boundary,
		CapturedAt:    capturedAt,
		SessionID:     ctx.SessionID,
		BeadID:        ctx.BeadID,
		AgentInfo:     ctx.AgentInfo,
		LeaseID:       ctx.LeaseID,
		Iteration:     ctx.Iteration,
		ObservedState: ctx.ObservedState,
		ExpectedStep:  ctx.ExpectedStep,
		KernlPID:      os.Getpid(),
		ChildPID:      ctx.ChildPID,
	}

	bead, leases, captureErrors := captureBeadSnapshotData(ctx, deps)

	snapshot := base
	snapshot.ObservedState = ctx.ObservedState
	if bead != nil && snapshot.ObservedState == "" {
		snapshot.ObservedState = bead.State
	}
	snapshot.Bead = bead
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
		slog.Error("dispatch forensics write failed", "boundary", boundary, "bead", ctx.BeadID, "error", err)
	} else {
		snapshotPath = pathBytes
	}

	logAudit := deps.LogAudit
	if logAudit == nil {
		logAudit = defaultLogAudit
	}
	logAudit(fmt.Sprintf("beat_snapshot_%s", boundary), map[string]any{
		"message":       fmt.Sprintf("Captured bead snapshot at boundary %s.", boundary),
		"repoPath":      ctx.RepoPath,
		"sessionId":     ctx.SessionID,
		"beadId":        ctx.BeadID,
		"knotsLeaseId":  ctx.LeaseID,
		"iteration":     ctx.Iteration,
		"observedState": snapshot.ObservedState,
		"expectedStep":  ctx.ExpectedStep,
		"snapshotPath":  snapshotPath,
		"captureErrors": snapshot.CaptureErrors,
	})

	return snapshot
}

func defaultLogAudit(event string, payload map[string]any) {
	slog.Info(event, "payload", payload)
}

func RunPostTurnForensics(pre, post BeadSnapshot, preSnapshotPath, postSnapshotPath string, signals *ClassifierSignals, deps *ForensicDeps) PostTurnForensicResult {
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
		BeadID:           post.BeadID,
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
		"message":            fmt.Sprintf("Dispatch forensic classified: %s.", classification.Category),
		"sessionId":          post.SessionID,
		"beadId":             post.BeadID,
		"knotsLeaseId":       post.LeaseID,
		"category":           string(classification.Category),
		"reasoning":          classification.Reasoning,
		"conflictingLeaseId": conflictingLeaseID,
		"preSnapshotPath":    preSnapshotPath,
		"postSnapshotPath":   postSnapshotPath,
	})

	return PostTurnForensicResult{
		Classified: true,
		BannerBody: body,
	}
}

type fsSnapshotWriter struct{}

func (f *fsSnapshotWriter) Write(snapshot BeadSnapshot) (string, error) {
	logRoot := resolveLogRoot()
	date := snapshot.CapturedAt[:10]
	p := SnapshotPath(SnapshotPathInput{
		LogRoot:    logRoot,
		Date:       date,
		SessionID:  snapshot.SessionID,
		BeadID:     snapshot.BeadID,
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
	if dir := os.Getenv("KERNL_LOG_ROOT"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "kernl", "logs")
	}
	return filepath.Join(home, ".kernl", "logs")
}
