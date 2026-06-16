package transport

var translatedMethods = map[string]bool{
	"turn/started":                              true,
	"turn/completed":                            true,
	"item/started":                              true,
	"item/completed":                            true,
	"item/agentMessage/delta":                   true,
	"item/reasoning/summaryTextDelta":           true,
	"item/reasoning/textDelta":                  true,
	"item/commandExecution/outputDelta":         true,
	"item/commandExecution/terminalInteraction": true,
}

func IsTranslatedMethod(method string) bool {
	return translatedMethods[method]
}

func asStr(v any) string {
	s, _ := v.(string)
	return s
}

func asMap(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func TranslateItemNotification(method string, params map[string]any) map[string]any {
	item := asMap(params["item"])
	if item == nil {
		return nil
	}
	if asStr(item["type"]) == "userMessage" {
		return nil
	}

	eventType := "item.started"
	if method == "item/completed" {
		eventType = "item.completed"
	}

	switch asStr(item["type"]) {
	case "commandExecution":
		return translateCommandExecution(item, eventType)
	case "agentMessage":
		return translateAgentMessage(item, eventType)
	case "reasoning":
		return translateReasoning(item, eventType)
	}
	return nil
}

func translateCommandExecution(item map[string]any, eventType string) map[string]any {
	command := extractCommand(item)
	output := asStr(item["output"])
	if output == "" {
		output = asStr(item["aggregatedOutput"])
	}
	result := map[string]any{
		"type": eventType,
		"item": map[string]any{
			"type":              "command_execution",
			"command":           command,
			"aggregated_output": output,
		},
	}
	if id := asStr(item["id"]); id != "" {
		result["item"].(map[string]any)["id"] = id
	}
	if status := asStr(item["status"]); status != "" {
		result["item"].(map[string]any)["status"] = status
	}
	return result
}

func extractCommand(item map[string]any) string {
	if cmd, ok := item["command"].(string); ok && cmd != "" {
		return cmd
	}
	call := asMap(item["call"])
	if call != nil {
		if cmd, ok := call["command"].(string); ok && cmd != "" {
			return cmd
		}
	}
	return ""
}

func translateAgentMessage(item map[string]any, eventType string) map[string]any {
	if eventType == "item.started" {
		result := map[string]any{
			"type": "item.started",
			"item": map[string]any{
				"type": "agent_message",
			},
		}
		if id := asStr(item["id"]); id != "" {
			result["item"].(map[string]any)["id"] = id
		}
		return result
	}
	text := collectText(item["fragments"])
	if text == "" {
		text = asStr(item["text"])
	}
	return map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"type": "agent_message",
			"text": text,
		},
	}
}

func translateReasoning(item map[string]any, eventType string) map[string]any {
	if eventType == "item.started" {
		return nil
	}
	text := collectText(item["summary"])
	if text == "" {
		text = collectText(item["summaryParts"])
	}
	if text == "" {
		text = collectText(item["content"])
	}
	if text == "" {
		return nil
	}
	return map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"type": "reasoning",
			"text": text,
		},
	}
}

func collectText(value any) string {
	arr, ok := value.([]any)
	if !ok {
		return ""
	}
	var parts []string
	for _, entry := range arr {
		obj := asMap(entry)
		if obj == nil {
			continue
		}
		if t := asStr(obj["text"]); t != "" {
			parts = append(parts, t)
		}
	}
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += "\n"
		}
		result += p
	}
	return result
}

func TranslateAgentMessageDelta(params map[string]any) map[string]any {
	text := asStr(params["delta"])
	if text == "" {
		text = asStr(params["text"])
	}
	if text == "" {
		return nil
	}
	item := map[string]any{
		"type": "agent_message",
	}
	if id := asStr(params["itemId"]); id != "" {
		item["id"] = id
	}
	return map[string]any{
		"type": "item.delta",
		"item": item,
		"text": text,
	}
}

func TranslateReasoningDelta(params map[string]any) map[string]any {
	text := asStr(params["delta"])
	if text == "" {
		text = asStr(params["text"])
	}
	if text == "" {
		return nil
	}
	item := map[string]any{
		"type": "reasoning",
	}
	if id := asStr(params["itemId"]); id != "" {
		item["id"] = id
	}
	return map[string]any{
		"type": "item.delta",
		"item": item,
		"text": text,
	}
}

func TranslateOutputDelta(params map[string]any) map[string]any {
	text := asStr(params["delta"])
	if text == "" {
		text = asStr(params["text"])
	}
	if text == "" {
		return nil
	}
	item := map[string]any{
		"type": "command_execution",
	}
	if id := asStr(params["itemId"]); id != "" {
		item["id"] = id
	}
	return map[string]any{
		"type": "item.delta",
		"item": item,
		"text": text,
	}
}

func TranslateTerminalInteraction(params map[string]any) map[string]any {
	itemID := asStr(params["itemId"])
	processID := asStr(params["processId"])
	stdin, _ := params["stdin"].(string)
	if itemID == "" && processID == "" && stdin == "" {
		return nil
	}
	item := map[string]any{
		"type": "command_execution",
	}
	if itemID != "" {
		item["id"] = itemID
	}
	result := map[string]any{
		"type":  "command_execution.terminal_interaction",
		"item":  item,
		"stdin": stdin,
	}
	if processID != "" {
		result["processId"] = processID
	}
	return result
}

type TurnResult struct {
	Event      map[string]any
	TurnFailed bool
}

func TranslateTurnCompleted(params map[string]any) TurnResult {
	turn := asMap(params["turn"])
	if turn != nil && asStr(turn["status"]) == "failed" {
		errorObj := asMap(turn["error"])
		msg := "Turn failed"
		if errorObj != nil {
			if m := asStr(errorObj["message"]); m != "" {
				msg = m
			}
		}
		return TurnResult{
			Event: map[string]any{
				"type":  "turn.failed",
				"error": map[string]any{"message": msg},
			},
			TurnFailed: true,
		}
	}
	return TurnResult{
		Event:      map[string]any{"type": "turn.completed"},
		TurnFailed: false,
	}
}
