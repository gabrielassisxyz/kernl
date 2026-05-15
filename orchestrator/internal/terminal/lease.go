package terminal

import (
	"fmt"
	"log/slog"

	"github.com/gabrielassisxyz/kernl/internal/backend"
)

type LeaseSource string

const (
	LeaseSourceTerminalManagerTake         LeaseSource = "terminal_manager_take"
	LeaseSourceStructuredPrepareTake       LeaseSource = "structured_prepare_take"
	LeaseSourceStructuredPreparePoll        LeaseSource = "structured_prepare_poll"
	LeaseSourceStructuredCompleteIteration  LeaseSource = "structured_complete_iteration"
	LeaseSourceStructuredRollbackIteration  LeaseSource = "structured_rollback_iteration"
	LeaseSourceDoctorActiveLeases          LeaseSource = "doctor_active_leases"
)

type EnsureLeaseInput struct {
	RepoPath         string
	Source           LeaseSource
	SessionID        string
	ExecutionLeaseID string
	BeatID           string
	ClaimedID        string
	InteractionType  string
	AgentInfo        *AgentInfo
}

type TerminateLeaseInput struct {
	RepoPath         string
	Source           LeaseSource
	SessionID        string
	ExecutionLeaseID string
	KnotsLeaseID     string
	BeatID           string
	ClaimedID        string
	InteractionType  string
	AgentInfo        *AgentInfo
	Reason           string
	Outcome          string
}

func leaseNickname(input *EnsureLeaseInput) string {
	parts := []string{"kernl", string(input.Source)}
	if input.SessionID != "" {
		parts = append(parts, input.SessionID)
	} else if input.ExecutionLeaseID != "" {
		parts = append(parts, input.ExecutionLeaseID)
	} else if input.BeatID != "" {
		parts = append(parts, input.BeatID)
	} else {
		parts = append(parts, "runtime")
	}
	nick := ""
	for i, p := range parts {
		if i > 0 {
			nick += ":"
		}
		nick += p
	}
	if len(nick) > 120 {
		nick = nick[:120]
	}
	return nick
}

func EnsureKnotsLease(knots *backend.KnotsBackend, input *EnsureLeaseInput) (string, error) {
	if input.BeatID != "" {
		slog.Debug("knots lease: pre_lease snapshot",
			"sessionId", input.SessionID, "beatId", input.BeatID, "repoPath", input.RepoPath)
	}

	slog.Info("knots lease: requesting lease",
		"source", input.Source, "beatId", input.BeatID,
		"sessionId", input.SessionID, "repoPath", input.RepoPath)

	opts := buildCreateLeaseOptions(input)

	result, err := knots.CreateLease(opts, input.RepoPath)
	if err != nil {
		slog.Error("KERNL DISPATCH FAILURE: knots lease create failed",
			"source", input.Source, "beatId", input.BeatID,
			"sessionId", input.SessionID, "error", err)
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: knots lease create failed for %s: %w", input.Source, err)
	}

	slog.Info("knots lease: created",
		"leaseId", result.ID, "source", input.Source,
		"beatId", input.BeatID, "sessionId", input.SessionID)

	if input.BeatID != "" {
		slog.Debug("knots lease: post_lease snapshot",
			"sessionId", input.SessionID, "beatId", input.BeatID,
			"leaseId", result.ID, "repoPath", input.RepoPath)
	}

	return result.ID, nil
}

func TerminateKnotsRuntimeLease(knots *backend.KnotsBackend, input *TerminateLeaseInput) {
	slog.Info("knots lease: terminating",
		"leaseId", input.KnotsLeaseID, "source", input.Source,
		"reason", input.Reason, "beatId", input.BeatID,
		"sessionId", input.SessionID)

	err := knots.TerminateLease(input.KnotsLeaseID, input.RepoPath)
	if err != nil {
		slog.Warn("knots lease: terminate failed",
			"leaseId", input.KnotsLeaseID, "source", input.Source,
			"error", err, "beatId", input.BeatID)
		return
	}

	slog.Info("knots lease: terminated",
		"leaseId", input.KnotsLeaseID, "source", input.Source,
		"reason", input.Reason, "beatId", input.BeatID)
}

func MakeReleaseKnotsLeaseFunc(entry *SessionEntry, knots *backend.KnotsBackend) ReleaseKnotsLeaseFunc {
	return func(reason, outcome string, data map[string]any) {
		entry.mu.Lock()
		leaseID := entry.KnotsLeaseID
		repoPath := entry.Session.RepoPath
		entry.mu.Unlock()

		if leaseID == "" {
			return
		}

		TerminateKnotsRuntimeLease(knots, &TerminateLeaseInput{
			RepoPath:     repoPath,
			Source:        LeaseSourceTerminalManagerTake,
			KnotsLeaseID: leaseID,
			Reason:       reason,
			Outcome:      outcome,
		})

		entry.ClearKnotsLease(reason)
	}
}

func EnsureAndSetKnotsLease(entry *SessionEntry, knots *backend.KnotsBackend, input *EnsureLeaseInput) error {
	leaseID, err := EnsureKnotsLease(knots, input)
	if err != nil {
		return err
	}

	entry.SetKnotsLease(leaseID, input.AgentInfo)
	entry.SetReleaseKnotsLease(MakeReleaseKnotsLeaseFunc(entry, knots))

	return nil
}

func buildCreateLeaseOptions(input *EnsureLeaseInput) backend.CreateLeaseOptions {
	opts := backend.CreateLeaseOptions{
		Nickname:  leaseNickname(input),
		Type:      "agent",
		AgentName: displayAgentName(input.AgentInfo),
		AgentType: agentTypeOrDefault(input.AgentInfo),
		Provider:  providerOrDefault(input.AgentInfo),
	}
	if input.AgentInfo != nil {
		opts.Model = input.AgentInfo.AgentModel
		opts.ModelVersion = input.AgentInfo.AgentVersion
	}
	return opts
}

func agentTypeOrDefault(info *AgentInfo) string {
	return "cli"
}

func providerOrDefault(info *AgentInfo) string {
	if info != nil {
		return info.AgentProvider
	}
	return ""
}

func displayAgentName(info *AgentInfo) string {
	if info == nil {
		return ""
	}
	return info.AgentName
}