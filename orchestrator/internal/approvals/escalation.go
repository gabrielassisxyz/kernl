package approvals

type Escalation struct {
	NotificationKey string `json:"notificationKey"`
	ApprovalID     string `json:"approvalId"`
	Status         string `json:"status"`
	Reason         string `json:"reason"`
}

func BuildEscalation(approval *ApprovalRequest) *Escalation {
	return &Escalation{
		NotificationKey: BuildApprovalLogicalKey(approval),
		ApprovalID:     approval.ID,
		Status:         "pending",
	}
}

func ExplainApprovalFailureReason(status string) string {
	switch status {
	case "reply_failed":
		return "The approval reply could not be delivered to the agent session"
	case "manual_required":
		return "The session ended before the approval could be processed; manual intervention required"
	case "rejected":
		return "The approval was rejected"
	default:
		return "Unknown approval failure: " + status
	}
}