package approvals

type ApprovalRequest struct {
	ID                string   `json:"id"`
	NotificationKey   string   `json:"notificationKey"`
	Status            string   `json:"status"`
	CreatedAt         string   `json:"createdAt"`
	UpdatedAt         string   `json:"updatedAt"`
	RepoPath          string   `json:"repoPath"`
	BeadID            string   `json:"beadId"`
	SessionID         string   `json:"sessionId"`
	Adapter           string   `json:"adapter"`
	Source            string   `json:"source"`
	ToolName          string   `json:"toolName"`
	SupportedActions  []string `json:"supportedActions"`
	Actionable        bool     `json:"actionable"`
}

type ApprovalFilter struct {
	RepoPath    string
	ActiveOnly  bool
	Status      string
	UpdatedSince string
}

func ExtractApprovalRequest(adapter string, raw map[string]any) (*ApprovalRequest, error) {
	return nil, nil
}

func BuildApprovalLogicalKey(approval *ApprovalRequest) string {
	return approval.SessionID + ":" + approval.BeadID + ":" + approval.ToolName
}

func FormatApprovalRequestBanner(approval *ApprovalRequest) string {
	return "KERNL APPROVAL REQUIRED tool=" + approval.ToolName + " bead=" + approval.BeadID
}