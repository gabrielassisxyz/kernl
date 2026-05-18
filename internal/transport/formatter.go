package transport

type TransportEvent struct {
	Type    string `json:"type"`
	Content string `json:"content,omitempty"`
	Source  string `json:"source,omitempty"`
}

func NormalizeEvent(adapter string, raw map[string]any) *TransportEvent {
	switch adapter {
	case "claude":
		return normalizeClaudeEvent(raw)
	case "codex":
		return normalizeCodexEvent(raw)
	case "copilot":
		return normalizeCopilotEvent(raw)
	case "gemini":
		return normalizeGeminiEvent(raw)
	case "opencode":
		return normalizeOpenCodeEvent(raw)
	default:
		return nil
	}
}

func normalizeClaudeEvent(raw map[string]any) *TransportEvent {
	t, _ := raw["type"].(string)
	if t == "result" {
		return &TransportEvent{Type: "turn_ended", Source: "claude"}
	}
	return &TransportEvent{Type: t, Source: "claude"}
}

func normalizeCodexEvent(raw map[string]any) *TransportEvent {
	t, _ := raw["type"].(string)
	if t == "turn/completed" {
		return &TransportEvent{Type: "turn_ended", Source: "codex"}
	}
	return &TransportEvent{Type: t, Source: "codex"}
}

func normalizeCopilotEvent(raw map[string]any) *TransportEvent {
	t, _ := raw["type"].(string)
	if t == "session.task_complete" {
		return &TransportEvent{Type: "turn_ended", Source: "copilot"}
	}
	return &TransportEvent{Type: t, Source: "copilot"}
}

func normalizeGeminiEvent(raw map[string]any) *TransportEvent {
	sr, _ := raw["stopReason"].(string)
	if sr == "end_turn" {
		return &TransportEvent{Type: "turn_ended", Source: "gemini"}
	}
	t, _ := raw["type"].(string)
	return &TransportEvent{Type: t, Source: "gemini"}
}

func normalizeOpenCodeEvent(raw map[string]any) *TransportEvent {
	t, _ := raw["type"].(string)
	if t == "session_idle" {
		return &TransportEvent{Type: "turn_ended", Source: "opencode"}
	}
	return &TransportEvent{Type: t, Source: "opencode"}
}