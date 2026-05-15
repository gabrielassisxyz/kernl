package terminal

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/session"
)

type ApprovalActionExecution struct {
	OK         bool                   `json:"ok"`
	HTTPStatus int                    `json:"httpStatus"`
	Record     *PendingApprovalRecord `json:"record,omitempty"`
	Error      string                 `json:"error,omitempty"`
	Code       string                 `json:"code,omitempty"`
}

func ApprovalStatusForAction(action ApprovalAction) ApprovalStatus {
	switch action {
	case ActionAccept:
		return ApprovalApproved
	case ActionAlwaysApprove:
		return ApprovalAlwaysApproved
	case ActionDecline:
		return ApprovalRejected
	default:
		return ApprovalPending
	}
}

func IsTerminalApprovalStatus(status ApprovalStatus) bool {
	switch status {
	case ApprovalApproved, ApprovalAlwaysApproved, ApprovalRejected, ApprovalDismissed:
		return true
	default:
		return false
	}
}

func NormalizeSupportedActions(actions []ApprovalAction) []ApprovalAction {
	if actions == nil {
		return []ApprovalAction{}
	}
	allowed := map[ApprovalAction]bool{
		ActionAccept:        true,
		ActionAlwaysApprove: true,
		ActionDecline:        true,
	}
	filtered := make([]ApprovalAction, 0, len(actions))
	for _, a := range actions {
		if allowed[a] {
			filtered = append(filtered, a)
		}
	}
	return filtered
}

func PerformApprovalAction(entry *SessionEntry, approvalID string, action ApprovalAction) ApprovalActionExecution {
	entry.mu.Lock()
	defer entry.mu.Unlock()

	record, ok := entry.PendingApprovals[approvalID]
	if !ok {
		return ApprovalActionExecution{
			OK:         false,
			HTTPStatus: 404,
			Error:      "Approval request not found",
		}
	}

	return executeApprovalAction(entry, record, action)
}

func executeApprovalAction(entry *SessionEntry, record *PendingApprovalRecord, action ApprovalAction) ApprovalActionExecution {
	unsupported := unsupportedReason(record, action, entry.ApprovalResponder)
	if unsupported != "" {
		markUnsupported(record, action, unsupported)
		return ApprovalActionExecution{
			OK:         false,
			HTTPStatus: 409,
			Record:     record,
			Code:       unsupported,
			Error:      fmt.Sprintf("Approval action is not supported: %s", unsupported),
		}
	}

	record.Status = ApprovalResponding
	record.FailureReason = ""
	now := time.Now().UnixMilli()
	record.UpdatedAt = now

	slog.Info("[terminal-manager] approval.action_sent",
		"approvalId", record.ApprovalID,
		"action", string(action),
	)

	result, err := entry.ApprovalResponder(record, action)
	if err != nil {
		return finishApprovalReply(entry, record, action, &ApprovalReplyResult{
			OK:     false,
			Status: "reply_failed",
			Reason: err.Error(),
		})
	}

	return finishApprovalReply(entry, record, action, result)
}

func unsupportedReason(record *PendingApprovalRecord, action ApprovalAction, responder ApprovalResponderFunc) string {
	actionStr := string(action)
	actionSupported := false
	for _, sa := range record.SupportedActions {
		if sa == actionStr {
			actionSupported = true
			break
		}
	}
	if !actionSupported {
		return "approval_action_not_supported"
	}
	if record.ReplyTarget == nil {
		return "approval_reply_target_missing"
	}
	if responder == nil {
		return "approval_responder_unavailable"
	}
	return ""
}

func markUnsupported(record *PendingApprovalRecord, action ApprovalAction, reason string) {
	record.Status = ApprovalUnsupported
	record.FailureReason = reason
	now := time.Now().UnixMilli()
	record.UpdatedAt = now

	slog.Info("[terminal-manager] approval.action_unsupported",
		"approvalId", record.ApprovalID,
		"action", string(action),
		"reason", reason,
	)
}

func finishApprovalReply(entry *SessionEntry, record *PendingApprovalRecord, action ApprovalAction, result *ApprovalReplyResult) ApprovalActionExecution {
	if !result.OK {
		status := "reply_failed"
		if result.Status == "unsupported" {
			status = "unsupported"
		}
		record.Status = ApprovalStatus(status)
		now := time.Now().UnixMilli()
		record.UpdatedAt = now
		reason := result.Reason
		if reason == "" {
			reason = result.Status
		}
		record.FailureReason = reason

		eventName := "approval.reply_failed"
		if status == "unsupported" {
			eventName = "approval.action_unsupported"
		}
		slog.Info("[terminal-manager] "+eventName,
			"approvalId", record.ApprovalID,
			"action", string(action),
			"reason", reason,
		)

		pushApprovalFailureEvent(entry, record, reason)

		code := "approval_reply_failed"
		if status == "unsupported" {
			code = "approval_action_not_supported"
		}
		httpStatus := 502
		if status == "unsupported" {
			httpStatus = 409
		}
		errMsg := reason
		if errMsg == "" {
			errMsg = "Approval reply failed"
		}
		return ApprovalActionExecution{
			OK:         false,
			HTTPStatus: httpStatus,
			Record:     record,
			Code:       code,
			Error:      errMsg,
		}
	}

	newStatus := ApprovalStatusForAction(action)
	record.Status = newStatus
	record.FailureReason = ""
	now := time.Now().UnixMilli()
	record.UpdatedAt = now

	slog.Info("[terminal-manager] approval.action_resolved",
		"approvalId", record.ApprovalID,
		"action", string(action),
	)

	return ApprovalActionExecution{
		OK:         true,
		HTTPStatus: 200,
		Record:     record,
	}
}

func pushApprovalFailureEvent(entry *SessionEntry, record *PendingApprovalRecord, reason string) {
	data := fmt.Sprintf(
		"\x1b[31m--- Approval reply failed for %s: %s ---\x1b[0m\n",
		record.ApprovalID,
		reason,
	)
	evt := session.TerminalEvent{
		Type:    "stderr",
		Content: data,
		BeatID:  entry.Session.BeatID,
		Time:    time.Now().UnixMilli(),
	}

	if len(entry.Buffer) >= MaxBuffer {
		entry.Buffer = entry.Buffer[1:]
	}
	entry.Buffer = append(entry.Buffer, evt)

	select {
	case entry.Events <- evt:
	default:
	}
}