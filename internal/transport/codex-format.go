package transport

import (
	"fmt"
	"strings"
)

func FormatCodexEvent(obj map[string]any) *FormattedEvent {
	t, _ := obj["type"].(string)
	if t == "" {
		return nil
	}

	switch t {
	case "turn.started":
		return &FormattedEvent{
			Text:     ansiDim + "▷ turn started" + ansiReset + "\n",
			IsDetail: true,
		}
	case "turn.completed":
		return &FormattedEvent{
			Text:     ansiDim + "▷ turn completed" + ansiReset + "\n",
			IsDetail: true,
		}
	case "turn.failed":
		return formatTurnFailed(obj)
	case "command_execution.terminal_interaction":
		return formatTerminalInteraction(obj)
	case "item.started", "item.completed", "item.delta":
		return formatItemEvent(t, obj)
	}
	return nil
}

func formatTurnFailed(obj map[string]any) *FormattedEvent {
	errorObj := asMap(obj["error"])
	msg := "Turn failed (no error message)"
	if errorObj != nil {
		if m := asStr(errorObj["message"]); m != "" {
			msg = m
		}
	}
	return &FormattedEvent{
		Text:     ansiRed + "✗ turn failed: " + msg + ansiReset + "\n",
		IsDetail: false,
	}
}

func formatTerminalInteraction(obj map[string]any) *FormattedEvent {
	item := asMap(obj["item"])
	itemID := "?"
	if item != nil {
		if id := asStr(item["id"]); id != "" {
			itemID = id
		}
	}
	processID := asStr(obj["processId"])
	if processID == "" {
		processID = "?"
	}
	stdin, _ := obj["stdin"].(string)
	stdinPart := " stdin=(empty)"
	if stdin != "" {
		clipped := clip(stdin, 60)
		stdinPart = fmt.Sprintf(" stdin=%q", clipped)
	}
	return &FormattedEvent{
		Text:     ansiDim + "↳ terminal interaction id=" + itemID + " pid=" + processID + stdinPart + ansiReset + "\n",
		IsDetail: true,
	}
}

func formatItemEvent(eventType string, obj map[string]any) *FormattedEvent {
	item := asMap(obj["item"])
	if item == nil {
		return nil
	}
	itemType := asStr(item["type"])
	if itemType == "" {
		return nil
	}

	switch itemType {
	case "command_execution":
		return formatCommandEvent(eventType, obj, item)
	case "agent_message":
		return formatAgentMessageEvent(eventType, obj, item)
	case "reasoning":
		return formatReasoningEvent(eventType, obj, item)
	}
	return nil
}

func formatCommandEvent(eventType string, obj map[string]any, item map[string]any) *FormattedEvent {
	if eventType == "item.started" {
		command := clip(asStr(item["command"]), 200)
		if command == "" {
			command = "(no command)"
		}
		return &FormattedEvent{
			Text:     ansiCyan + "▶ " + command + ansiReset + "\n",
			IsDetail: true,
		}
	}
	if eventType == "item.delta" {
		text := asStr(obj["text"])
		if text == "" {
			return nil
		}
		return &FormattedEvent{
			Text:     ansiDim + text + ansiReset,
			IsDetail: true,
		}
	}
	output := asStr(item["aggregated_output"])
	status := asStr(item["status"])
	var statusTag string
	if status != "" && status != "completed" {
		statusTag = " " + ansiYellow + "[" + status + "]" + ansiReset
	}
	if output == "" {
		return &FormattedEvent{
			Text:     ansiDim + "↳ command finished" + statusTag + ansiReset + "\n",
			IsDetail: true,
		}
	}
	trimmed := clip(output, 1500)
	suffix := "\n"
	if statusTag != "" {
		suffix = statusTag + "\n"
	}
	return &FormattedEvent{
		Text:     ansiDim + trimmed + ansiReset + suffix,
		IsDetail: true,
	}
}

func formatAgentMessageEvent(eventType string, obj map[string]any, item map[string]any) *FormattedEvent {
	if eventType == "item.started" {
		return nil
	}
	if eventType == "item.delta" {
		text := asStr(obj["text"])
		if text == "" {
			return nil
		}
		return &FormattedEvent{Text: text, IsDetail: false}
	}
	text := asStr(item["text"])
	if text == "" {
		return nil
	}
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	return &FormattedEvent{Text: text, IsDetail: false}
}

func formatReasoningEvent(eventType string, obj map[string]any, item map[string]any) *FormattedEvent {
	if eventType == "item.started" {
		return nil
	}
	var text string
	if eventType == "item.delta" {
		text = asStr(obj["text"])
	} else {
		text = asStr(item["text"])
	}
	if text == "" {
		return nil
	}
	suffix := "\n"
	if strings.HasSuffix(text, "\n") {
		suffix = ""
	}
	return &FormattedEvent{
		Text:     ansiMagenta + text + ansiReset + suffix,
		IsDetail: true,
	}
}