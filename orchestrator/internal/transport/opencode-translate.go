package transport



// TranslateOpenCodePart converts an OpenCode message part into
// one or more normalized event maps. Tool parts with completed
// state produce both a tool_use and tool_result. Unknown parts
// yield zero events.
//
// Usage: for _, ev := range TranslateOpenCodePart(part) { ... }
func TranslateOpenCodePart(part map[string]any) []map[string]any {
	t, _ := part["type"].(string)

	switch t {
	case "step-start":
		return []map[string]any{{"type": "step_start"}}

	case "text":
		text, _ := part["text"].(string)
		return []map[string]any{{
			"type": "text",
			"part": map[string]any{"text": text},
		}}

	case "step-finish":
		reason := "stop"
		if r, ok := part["reason"].(string); ok && r != "" {
			reason = r
		}
		return []map[string]any{{
			"type": "step_finish",
			"part": map[string]any{"reason": reason},
		}}

	case "tool":
		return translateToolPart(part)

	case "reasoning":
		text, _ := part["text"].(string)
		return []map[string]any{{"type": "reasoning", "text": text}}

	case "file":
		filename, _ := part["filename"].(string)
		result := map[string]any{"type": "file", "filename": filename}
		if mime, ok := part["mime"].(string); ok {
			result["mime"] = mime
		}
		if source, ok := part["source"].(string); ok {
			result["source"] = source
		}
		return []map[string]any{result}

	case "snapshot":
		snap, _ := part["snapshot"].(string)
		return []map[string]any{{"type": "snapshot", "snapshot": snap}}

	default:
		perm := permissionEnvelope(part)
		if perm != nil {
			return []map[string]any{perm}
		}
		return nil
	}
}

func translateToolPart(part map[string]any) []map[string]any {
	state, _ := part["state"].(map[string]any)
	if state == nil {
		state = map[string]any{}
	}

	id := firstStr(part, "id", "callID", "callId")
	name := firstStr(part, "tool", "name")
	if name == "" {
		name = "tool"
	}
	input, _ := state["input"].(map[string]any)
	if input == nil {
		input = map[string]any{}
	}

	toolUse := map[string]any{
		"type":  "tool_use",
		"name":  name,
		"input": input,
	}
	if id != "" {
		toolUse["id"] = id
	}
	if status, ok := state["status"].(string); ok {
		toolUse["status"] = status
	}

	var out []map[string]any
	out = append(out, toolUse)

	status, _ := state["status"].(string)
	if _, hasOutput := state["output"]; hasOutput && status != "pending" {
		toolResult := map[string]any{
			"type":    "tool_result",
			"content": state["output"],
		}
		if id != "" {
			toolResult["tool_use_id"] = id
		}
		if status != "" {
			toolResult["status"] = status
		}
		out = append(out, toolResult)
	}

	return out
}

func firstStr(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func toObj(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func findPartContainer(event map[string]any) map[string]any {
	if p := toObj(event["properties"]); p != nil {
		return p
	}
	if d := toObj(event["data"]); d != nil {
		return d
	}
	return event
}

func extractPart(event map[string]any) map[string]any {
	container := findPartContainer(event)
	if candidate := toObj(container["part"]); candidate != nil {
		if t, ok := candidate["type"].(string); ok && t != "" {
			return candidate
		}
	}
	if candidate := toObj(event["part"]); candidate != nil {
		if t, ok := candidate["type"].(string); ok && t != "" {
			return candidate
		}
	}
	return nil
}

func extractInfo(event map[string]any) map[string]any {
	container := findPartContainer(event)
	if info := toObj(container["info"]); info != nil {
		return info
	}
	return toObj(event["info"])
}

func permissionEnvelope(event map[string]any) map[string]any {
	t, _ := event["type"].(string)
	if t == "permission.asked" || t == "permission.updated" {
		return event
	}
	if name := firstStr(event, "event", "name"); name == "permission.asked" || name == "permission.updated" {
		result := make(map[string]any, len(event)+1)
		for k, v := range event {
			result[k] = v
		}
		result["type"] = name
		return result
	}
	part := toObj(event["part"])
	if part != nil {
		pt := firstStr(part, "type", "event", "name")
		if pt == "permission.asked" || pt == "permission.updated" {
			result := make(map[string]any, len(event)+1)
			for k, v := range event {
				result[k] = v
			}
			result["type"] = pt
			return result
		}
	}
	return nil
}

// TranslateOpenCodeEvent translates an OpenCode SSE envelope into
// normalized Foolery-shaped event objects. Returns 0, 1, or 2 events
// (tool parts can produce tool_use + tool_result). Unknown or
// non-object inputs return nil.
func TranslateOpenCodeEvent(value any) []map[string]any {
	event, _ := value.(map[string]any)
	if event == nil {
		return nil
	}

	if perm := permissionEnvelope(event); perm != nil {
		return []map[string]any{perm}
	}

	t, _ := event["type"].(string)
	if t == "" {
		return nil
	}

	switch t {
	case "message.part.updated":
		part := extractPart(event)
		if part == nil {
			return nil
		}
		return TranslateOpenCodePart(part)

	case "message.updated":
		info := extractInfo(event)
		if info == nil {
			info = map[string]any{}
		}
		return []map[string]any{{"type": "message_updated", "info": info}}

	case "step.updated":
		container := findPartContainer(event)
		step := toObj(container["step"])
		if step == nil {
			step = toObj(event["step"])
		}
		if step == nil {
			step = map[string]any{}
		}
		return []map[string]any{{"type": "step_updated", "step": step}}

	case "session.idle":
		container := findPartContainer(event)
		sid := firstStr(container, "sessionID", "sessionId")
		if sid == "" {
			sid = firstStr(event, "sessionID", "sessionId")
		}
		return []map[string]any{{"type": "session_idle", "sessionID": sid}}

	case "session.status":
		container := findPartContainer(event)
		status := toObj(container["status"])
		if status == nil {
			status = toObj(event["status"])
		}
		statusType, _ := status["type"].(string)
		if statusType != "idle" {
			return nil
		}
		sid := firstStr(container, "sessionID", "sessionId")
		if sid == "" {
			sid = firstStr(event, "sessionID", "sessionId")
		}
		return []map[string]any{{"type": "session_idle", "sessionID": sid}}

	case "session.error":
		container := findPartContainer(event)
		errorObj := toObj(container["error"])
		if errorObj == nil {
			errorObj = toObj(event["error"])
		}
		if errorObj == nil {
			errorObj = map[string]any{}
		}
		msg := firstStr(errorObj, "message", "name")
		if msg == "" {
			msg = "OpenCode session error"
		}
		return []map[string]any{{
			"type":    "session_error",
			"error":   errorObj,
			"message": msg,
		}}
	}

	return nil
}

// HasOpenCodeMessagePayload returns whether a response object contains
// processable OpenCode content (parts, events collections, or a
// direct permission event).
func HasOpenCodeMessagePayload(resp map[string]any) bool {
	if _, ok := resp["parts"]; ok {
		return true
	}
	for _, key := range []string{"events", "stream", "items"} {
		if _, ok := resp[key]; ok {
			return true
		}
	}
	if len(TranslateOpenCodeEvent(resp)) > 0 {
		return true
	}
	return false
}

// TranslateOpenCodeResponse translates a full OpenCode message
// response (with parts, events, stream, items, plus any top-level
// envelopes) into a flat slice of normalized events in order.
func TranslateOpenCodeResponse(resp map[string]any) []map[string]any {
	var events []map[string]any

	if parts, ok := resp["parts"].([]any); ok {
		for _, p := range parts {
			part, ok := p.(map[string]any)
			if !ok {
				continue
			}
			events = append(events, TranslateOpenCodePart(part)...)
		}
	}

	for _, key := range []string{"events", "stream", "items"} {
		collection, ok := resp[key].([]any)
		if !ok {
			continue
		}
		for _, e := range collection {
			for _, ev := range TranslateOpenCodeEvent(e) {
				events = append(events, ev)
			}
		}
	}

	for _, ev := range TranslateOpenCodeEvent(resp) {
		events = append(events, ev)
	}

	return events
}

