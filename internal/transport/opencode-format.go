package transport

import (
	"strings"
)

// FormattedEvent is a terminal-renderable event produced by
// FormatOpenCodeEvent. Text contains ANSI escape codes for
// color; IsDetail marks events that should render dimmed.
type FormattedEvent struct {
	Text     string
	IsDetail bool
}

const (
	ansiReset   = "\x1b[0m"
	ansiDim     = "\x1b[90m"
	ansiCyan    = "\x1b[36m"
	ansiRed     = "\x1b[31m"
	ansiMagenta = "\x1b[35m"
	ansiYellow  = "\x1b[33m"
)

// FormatOpenCodeEvent renders a translated OpenCode event for
// terminal display. Returns nil for event types it does not
// handle or when the event content is empty.
//
// Usage: if ev := FormatOpenCodeEvent(event); ev != nil { ... }
func FormatOpenCodeEvent(obj map[string]any) *FormattedEvent {
	t, _ := obj["type"].(string)
	if t == "" {
		return nil
	}

	switch t {
	case "reasoning":
		return formatReasoning(obj)
	case "step_updated":
		return formatStepUpdated(obj)
	case "session_idle":
		return formatSessionIdle(obj)
	case "session_error":
		return formatSessionError(obj)
	case "file":
		return formatFile(obj)
	case "snapshot":
		return formatSnapshot(obj)
	case "message_updated":
		return formatMessageUpdated(obj)
	default:
		return nil
	}
}

func formatReasoning(obj map[string]any) *FormattedEvent {
	text, _ := obj["text"].(string)
	if text == "" {
		return nil
	}
	trailing := "\n"
	if strings.HasSuffix(text, "\n") {
		trailing = ""
	}
	return &FormattedEvent{
		Text:     ansiMagenta + text + ansiReset + trailing,
		IsDetail: true,
	}
}

func formatStepUpdated(obj map[string]any) *FormattedEvent {
	step, _ := obj["step"].(map[string]any)
	if step == nil {
		step = map[string]any{}
	}
	name, _ := step["name"].(string)
	status, _ := step["status"].(string)
	if status == "" {
		status, _ = step["state"].(string)
	}
	if name == "" && status == "" {
		return nil
	}
	parts := []string{name, status}
	var label string
	for _, p := range parts {
		if p != "" {
			if label != "" {
				label += " "
			}
			label += p
		}
	}
	return &FormattedEvent{
		Text:     ansiDim + "▷ step " + label + ansiReset + "\n",
		IsDetail: true,
	}
}

func formatSessionIdle(obj map[string]any) *FormattedEvent {
	sid, _ := obj["sessionID"].(string)
	tag := ""
	if sid != "" {
		tag = " " + sid
	}
	return &FormattedEvent{
		Text:     ansiDim + "▷ session idle" + tag + ansiReset + "\n",
		IsDetail: true,
	}
}

func formatSessionError(obj map[string]any) *FormattedEvent {
	msg, _ := obj["message"].(string)
	if msg == "" {
		msg = "OpenCode session error"
	}
	return &FormattedEvent{
		Text:     ansiRed + "✗ " + msg + ansiReset + "\n",
		IsDetail: false,
	}
}

func formatFile(obj map[string]any) *FormattedEvent {
	filename, _ := obj["filename"].(string)
	if filename == "" {
		return nil
	}
	mime, _ := obj["mime"].(string)
	mimeTag := ""
	if mime != "" {
		mimeTag = " " + ansiDim + "(" + mime + ")" + ansiReset
	}
	return &FormattedEvent{
		Text:     ansiCyan + "📎 " + filename + ansiReset + mimeTag + "\n",
		IsDetail: true,
	}
}

func clip(text string, max int) string {
	if len(text) <= max {
		return text
	}
	return text[:max] + "..."
}

func formatSnapshot(obj map[string]any) *FormattedEvent {
	snap, _ := obj["snapshot"].(string)
	if snap == "" {
		return nil
	}
	return &FormattedEvent{
		Text:     ansiDim + "↳ snapshot " + clip(snap, 64) + ansiReset + "\n",
		IsDetail: true,
	}
}

func formatMessageUpdated(obj map[string]any) *FormattedEvent {
	info, _ := obj["info"].(map[string]any)
	if info == nil {
		return nil
	}
	timeObj, _ := info["time"].(map[string]any)
	if timeObj == nil {
		return nil
	}
	if _, ok := timeObj["completed"]; !ok {
		return nil
	}
	return &FormattedEvent{
		Text:     ansiDim + "▷ turn complete" + ansiReset + "\n",
		IsDetail: true,
	}
}
