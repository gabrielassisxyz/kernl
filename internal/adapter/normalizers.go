package adapter

import (
	"encoding/json"
	"strings"
)

type Normalizer func(parsed json.RawMessage) (map[string]any, bool)

func CreateLineNormalizer(dialect AgentDialect) Normalizer {
	switch dialect {
	case DialectClaude:
		return ClaudeNormalizer()
	case DialectCodex:
		return CodexNormalizer()
	case DialectCopilot:
		return CopilotNormalizer()
	case DialectOpenCode:
		return OpenCodeNormalizer()
	case DialectGemini:
		return GeminiNormalizer()
	default:
		return CodexNormalizer()
	}
}

func ClaudeNormalizer() Normalizer {
	return func(parsed json.RawMessage) (map[string]any, bool) {
		var obj map[string]any
		if err := json.Unmarshal(parsed, &obj); err != nil {
			return nil, false
		}
		if obj == nil {
			return nil, false
		}
		return obj, true
	}
}

type geminiState struct {
	accumulatedText string
}

func GeminiNormalizer() Normalizer {
	s := &geminiState{}
	return func(parsed json.RawMessage) (map[string]any, bool) {
		var obj map[string]any
		if err := json.Unmarshal(parsed, &obj); err != nil {
			return nil, false
		}
		t, _ := obj["type"].(string)
		if t == "" {
			return nil, false
		}
		if t == "init" {
			return nil, false
		}
		if t == "message" {
			role, _ := obj["role"].(string)
			if role == "user" {
				return nil, false
			}
			content, _ := obj["content"].(string)
			if content != "" {
				if s.accumulatedText != "" {
					s.accumulatedText += "\n"
				}
				s.accumulatedText += content
			}
			return map[string]any{
				"type":    "assistant",
				"message": map[string]any{"content": []any{map[string]any{"type": "text", "text": content}}},
			}, true
		}
		if t == "result" {
			status, _ := obj["status"].(string)
			isError := status != "success"
			result := s.accumulatedText
			if result == "" && isError {
				result = "Gemini error"
			}
			if !isError {
				result = s.accumulatedText
			}
			return map[string]any{
				"type":     "result",
				"result":   result,
				"is_error": isError,
			}, true
		}
		return nil, false
	}
}

type openCodeState struct {
	accumulatedText string
}

func OpenCodeNormalizer() Normalizer {
	s := &openCodeState{}
	return func(parsed json.RawMessage) (map[string]any, bool) {
		var obj map[string]any
		if err := json.Unmarshal(parsed, &obj); err != nil {
			return nil, false
		}
		t, _ := obj["type"].(string)

		if t == "step_start" {
			return nil, false
		}
		if t == "text" {
			part, _ := obj["part"].(map[string]any)
			text, _ := part["text"].(string)
			if s.accumulatedText != "" {
				s.accumulatedText += "\n"
			}
			s.accumulatedText += text
			return map[string]any{
				"type":    "assistant",
				"message": map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}},
			}, true
		}
		if t == "step_finish" {
			part, _ := obj["part"].(map[string]any)
			reason, _ := part["reason"].(string)
			if reason != "error" {
				return nil, false
			}
			return map[string]any{
				"type":     "result",
				"result":   s.accumulatedText,
				"is_error": true,
			}, true
		}
		if t == "session_idle" {
			result := map[string]any{
				"type":     "result",
				"result":   s.accumulatedText,
				"is_error": false,
			}
			s.accumulatedText = ""
			return result, true
		}
		if t == "tool_use" {
			id, _ := obj["id"].(string)
			name, _ := obj["name"].(string)
			if name == "" {
				name = "tool"
			}
			input, _ := obj["input"].(map[string]any)
			if input == nil {
				input = map[string]any{}
			}
			content := map[string]any{"type": "tool_use", "name": name, "input": input}
			if id != "" {
				content["id"] = id
			}
			return map[string]any{
				"type":    "assistant",
				"message": map[string]any{"content": []any{content}},
			}, true
		}
		if t == "tool_result" {
			toolUseID, _ := obj["tool_use_id"].(string)
			raw := obj["content"]
			var contentStr string
			switch v := raw.(type) {
			case string:
				contentStr = v
			case nil:
				contentStr = ""
			default:
				b, _ := json.Marshal(v)
				contentStr = string(b)
			}
			result := map[string]any{"type": "tool_result", "content": contentStr}
			if toolUseID != "" {
				result["tool_use_id"] = toolUseID
			}
			return map[string]any{
				"type":    "user",
				"message": map[string]any{"content": []any{result}},
			}, true
		}
		if t == "reasoning" {
			text, _ := obj["text"].(string)
			if text == "" {
				return nil, false
			}
			return map[string]any{
				"type":  "stream_event",
				"event": map[string]any{"type": "content_block_delta", "delta": map[string]any{"type": "text_delta", "text": text}},
			}, true
		}
		if t == "session_error" {
			msg, _ := obj["message"].(string)
			if msg == "" {
				msg = "OpenCode session error"
			}
			return map[string]any{
				"type":     "result",
				"result":   s.accumulatedText,
				"is_error": true,
			}, true
		}
		return nil, false
	}
}

type copilotState struct {
	accumulatedText    string
	streamedMessageIDs map[string]bool
}

func CopilotNormalizer() Normalizer {
	s := &copilotState{
		streamedMessageIDs: make(map[string]bool),
	}
	return func(parsed json.RawMessage) (map[string]any, bool) {
		var obj map[string]any
		if err := json.Unmarshal(parsed, &obj); err != nil {
			return nil, false
		}
		t, _ := obj["type"].(string)
		if t == "" {
			return nil, false
		}
		data, _ := obj["data"].(map[string]any)

		if t == "assistant.message_delta" {
			messageID, _ := data["messageId"].(string)
			delta, _ := data["deltaContent"].(string)
			if delta == "" {
				return nil, false
			}
			if messageID != "" {
				s.streamedMessageIDs[messageID] = true
			}
			s.accumulatedText += delta
			return map[string]any{
				"type":  "stream_event",
				"event": map[string]any{"type": "content_block_delta", "delta": map[string]any{"type": "text_delta", "text": delta}},
			}, true
		}
		if t == "assistant.message" {
			return normalizeCopilotMessage(data, s), true
		}
		if t == "user_input.requested" {
			return normalizeCopilotUserInput(data), true
		}
		if t == "session.task_complete" {
			dataInner, _ := obj["data"].(map[string]any)
			summary, _ := dataInner["summary"].(string)
			success, _ := dataInner["success"].(bool)
			result := s.accumulatedText
			if result == "" {
				result = summary
			}
			if result == "" && !success {
				result = "Task failed"
			}
			return map[string]any{
				"type":     "result",
				"result":   result,
				"is_error": !success,
			}, true
		}
		if t == "session.error" {
			dataInner, _ := obj["data"].(map[string]any)
			msg, _ := dataInner["message"].(string)
			if msg == "" {
				msg = "Session error"
			}
			return map[string]any{
				"type":     "result",
				"result":   msg,
				"is_error": true,
			}, true
		}
		return nil, false
	}
}

func normalizeCopilotMessage(data map[string]any, s *copilotState) map[string]any {
	if data == nil {
		return nil
	}
	messageID, _ := data["messageId"].(string)
	content, _ := data["content"].(string)
	var blocks []any

	toolRequests, _ := data["toolRequests"].([]any)
	for _, raw := range toolRequests {
		req, _ := raw.(map[string]any)
		if req == nil {
			continue
		}
		name, _ := req["name"].(string)
		if name == "" {
			continue
		}
		toolUseID, _ := req["toolCallId"].(string)
		input, _ := req["arguments"].(map[string]any)
		if input == nil {
			input = map[string]any{}
		}
		block := map[string]any{"type": "tool_use", "name": name, "input": input}
		if toolUseID != "" {
			block["id"] = toolUseID
		}
		blocks = append(blocks, block)
	}

	if content != "" && (messageID == "" || !s.streamedMessageIDs[messageID]) {
		if s.accumulatedText != "" {
			s.accumulatedText += "\n"
		}
		s.accumulatedText += content
		textBlock := map[string]any{"type": "text", "text": content}
		blocks = append([]any{textBlock}, blocks...)
	}
	if len(blocks) == 0 {
		return nil
	}
	return map[string]any{"type": "assistant", "message": map[string]any{"content": blocks}}
}

func normalizeCopilotUserInput(data map[string]any) map[string]any {
	question, _ := data["question"].(string)
	if question == "" {
		return nil
	}
	var options []any
	rawChoices, _ := data["choices"].([]any)
	for _, c := range rawChoices {
		label, _ := c.(string)
		if strings.TrimSpace(label) != "" {
			options = append(options, map[string]any{"label": label})
		}
	}
	toolUseID, _ := data["toolCallId"].(string)
	if toolUseID == "" {
		toolUseID, _ = data["requestId"].(string)
	}
	result := map[string]any{
		"type": "tool_use",
		"name": "AskUserQuestion",
		"input": map[string]any{
			"questions": []any{map[string]any{"question": question, "options": options}},
		},
	}
	if toolUseID != "" {
		result["id"] = toolUseID
	}
	return map[string]any{"type": "assistant", "message": map[string]any{"content": []any{result}}}
}

type codexState struct {
	accumulatedText string
}

func CodexNormalizer() Normalizer {
	s := &codexState{}
	return func(parsed json.RawMessage) (map[string]any, bool) {
		var obj map[string]any
		if err := json.Unmarshal(parsed, &obj); err != nil {
			return nil, false
		}
		t, _ := obj["type"].(string)

		if t == "thread.started" || t == "turn.started" {
			return nil, false
		}
		if t == "item.completed" {
			item, _ := obj["item"].(map[string]any)
			if item == nil {
				return nil, false
			}
			result := normalizeCodexItemCompleted(item, s)
			if result == nil {
				return nil, false
			}
			return result, true
		}
		if t == "item.started" {
			item, _ := obj["item"].(map[string]any)
			if item == nil {
				return nil, false
			}
			itemType, _ := item["type"].(string)
			if itemType == "command_execution" {
				cmd, _ := item["command"].(string)
				return map[string]any{
					"type":    "assistant",
					"message": map[string]any{"content": []any{map[string]any{"type": "tool_use", "name": "Bash", "input": map[string]any{"command": cmd}}}},
				}, true
			}
			return nil, false
		}
		result := normalizeCodexTerminal(obj, s)
		if result == nil {
			return nil, false
		}
		return result, true
	}
}

func normalizeCodexItemCompleted(item map[string]any, s *codexState) map[string]any {
	itemType, _ := item["type"].(string)
	if itemType == "agent_message" {
		text, _ := item["text"].(string)
		if s.accumulatedText != "" {
			s.accumulatedText += "\n"
		}
		s.accumulatedText += text
		return map[string]any{
			"type":    "assistant",
			"message": map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}},
		}
	}
	if itemType == "reasoning" {
		text, _ := item["text"].(string)
		return map[string]any{
			"type":  "stream_event",
			"event": map[string]any{"type": "content_block_delta", "delta": map[string]any{"type": "text_delta", "text": text}},
		}
	}
	if itemType == "command_execution" {
		output, _ := item["aggregated_output"].(string)
		return map[string]any{
			"type":    "user",
			"message": map[string]any{"content": []any{map[string]any{"type": "tool_result", "content": output}}},
		}
	}
	return nil
}

func normalizeCodexTerminal(obj map[string]any, s *codexState) map[string]any {
	t, _ := obj["type"].(string)
	if t == "turn.completed" {
		status, _ := obj["status"].(string)
		if status == "failed" {
			errObj, _ := obj["error"].(map[string]any)
			msg, _ := errObj["message"].(string)
			if msg == "" {
				msg = "Turn failed"
			}
			return map[string]any{"type": "result", "result": msg, "is_error": true}
		}
		return map[string]any{"type": "result", "result": s.accumulatedText, "is_error": false}
	}
	if t == "turn.failed" {
		errObj, _ := obj["error"].(map[string]any)
		msg, _ := errObj["message"].(string)
		if msg == "" {
			msg = "Turn failed"
		}
		return map[string]any{"type": "result", "result": msg, "is_error": true}
	}
	if t == "error" {
		msg, _ := obj["message"].(string)
		if msg == "" {
			msg = "Unknown error"
		}
		return map[string]any{"type": "result", "result": msg, "is_error": true}
	}
	return nil
}
