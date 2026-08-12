package approvals

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

// The exact messages claude 2.1.228 sends, captured from a real one-shot run
// against a stub bridge. They are transcribed rather than paraphrased: this
// test is the contract with the agent, and a paraphrase would only prove the
// bridge agrees with itself.
const (
	realInitialize  = `{"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{"roots":{"listChanged":true},"elicitation":{}},"clientInfo":{"name":"claude-code","title":"Claude Code","version":"2.1.228"}},"jsonrpc":"2.0","id":0}`
	realInitialized = `{"method":"notifications/initialized","jsonrpc":"2.0"}`
	realToolsList   = `{"method":"tools/list","jsonrpc":"2.0","id":1}`
	realToolsCall   = `{"method":"tools/call","params":{"name":"ask","arguments":{"tool_name":"Write","input":{"file_path":"/tmp/hello.txt","content":"hi\n"},"tool_use_id":"toolu_016PBEhbyRSDU47HArScE6rM"},"_meta":{"claudecode/toolUseId":"toolu_016PBEhbyRSDU47HArScE6rM","progressToken":2}},"jsonrpc":"2.0","id":2}`
)

type rpcReply struct {
	ID     json.RawMessage `json:"id"`
	Result struct {
		ProtocolVersion string           `json:"protocolVersion"`
		ServerInfo      map[string]any   `json:"serverInfo"`
		Tools           []map[string]any `json:"tools"`
		Content         []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"result"`
	Error *mcpError `json:"error"`
}

type verdict struct {
	Behavior     string         `json:"behavior"`
	Message      string         `json:"message"`
	UpdatedInput map[string]any `json:"updatedInput"`
}

// serveLines runs the bridge over a fixed script and returns its replies.
func serveLines(t *testing.T, s *Store, answer func(*Store), lines ...string) []rpcReply {
	t.Helper()
	in := strings.NewReader(strings.Join(lines, "\n") + "\n")
	pr, pw := io.Pipe()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err := ServeMCP(context.Background(), s, "claude", fastContext(), in, pw)
		pw.CloseWithError(err)
	}()
	if answer != nil {
		go answer(s)
	}

	var replies []rpcReply
	dec := json.NewDecoder(pr)
	for {
		var r rpcReply
		if err := dec.Decode(&r); err != nil {
			break
		}
		replies = append(replies, r)
	}
	wg.Wait()
	return replies
}

func TestMCPHandshakeMatchesTheAgentsScript(t *testing.T) {
	s := newTestStore(t)
	replies := serveLines(t, s, nil, realInitialize, realInitialized, realToolsList)

	if len(replies) != 2 {
		t.Fatalf("want 2 replies (initialize, tools/list) - a notification is never answered - got %d", len(replies))
	}
	if replies[0].Result.ProtocolVersion != "2025-11-25" {
		t.Errorf("the server must agree with the client's protocol version, got %q", replies[0].Result.ProtocolVersion)
	}
	if replies[0].Result.ServerInfo["name"] != mcpServerName {
		t.Errorf("serverInfo.name must be %q, got %v", mcpServerName, replies[0].Result.ServerInfo["name"])
	}
	if len(replies[1].Result.Tools) != 1 || replies[1].Result.Tools[0]["name"] != mcpToolName {
		t.Errorf("tools/list must advertise exactly %q, got %+v", mcpToolName, replies[1].Result.Tools)
	}
}

func TestMCPAllowedCallEchoesTheInputBack(t *testing.T) {
	s := newTestStore(t)
	replies := serveLines(t, s, answerWith(ActionAllow, ""), realInitialize, realToolsCall)

	v := lastVerdict(t, replies)
	if v.Behavior != "allow" {
		t.Fatalf("want behavior allow, got %+v", v)
	}
	// claude runs the tool with updatedInput, so dropping it would silently
	// run a different call than the human approved.
	if v.UpdatedInput["file_path"] != "/tmp/hello.txt" || v.UpdatedInput["content"] != "hi\n" {
		t.Errorf("the approved input must come back verbatim, got %+v", v.UpdatedInput)
	}
}

func TestMCPDeniedCallCarriesTheReason(t *testing.T) {
	s := newTestStore(t)
	replies := serveLines(t, s, answerWith(ActionDeny, "not on this branch"), realInitialize, realToolsCall)

	v := lastVerdict(t, replies)
	if v.Behavior != "deny" {
		t.Fatalf("want behavior deny, got %+v", v)
	}
	if v.Message != "not on this branch" {
		t.Errorf("the agent must be told why, got %q", v.Message)
	}
}

// A bridge that cannot raise a gate must deny. Reporting the failure as an
// allow would turn every outage of the approval store into blanket permission.
func TestMCPUnraisableGateDenies(t *testing.T) {
	s := newTestStore(t)
	badCall := `{"method":"tools/call","params":{"name":"ask","arguments":{"input":{}}},"jsonrpc":"2.0","id":9}`
	replies := serveLines(t, s, nil, realInitialize, badCall)

	v := lastVerdict(t, replies)
	if v.Behavior != "deny" {
		t.Fatalf("a gate that could not be raised must deny, got %+v", v)
	}
}

func TestMCPUnknownMethodIsRefusedNotAnsweredEmpty(t *testing.T) {
	s := newTestStore(t)
	replies := serveLines(t, s, nil, `{"method":"resources/list","jsonrpc":"2.0","id":7}`)

	if len(replies) != 1 || replies[0].Error == nil {
		t.Fatalf("an unimplemented method must return an error, got %+v", replies)
	}
	if !strings.Contains(replies[0].Error.Message, "resources/list") {
		t.Errorf("the error must name what was asked for, got %q", replies[0].Error.Message)
	}
}

// Two gates in one turn must not queue behind each other: the second would be
// indistinguishable from a hung bridge.
//
// The answerer deliberately waits for BOTH requests to be parked at once
// before answering either. A bridge that handles tools/call serially never
// parks the second until the first is answered, so that condition is never
// reached and both gates expire into denials - which is what makes this test
// able to fail.
func TestMCPConcurrentCallsAreNotSerialized(t *testing.T) {
	s := newTestStore(t)
	second := strings.Replace(realToolsCall, `"id":2`, `"id":3`, 1)
	second = strings.Replace(second, `"Write"`, `"Bash"`, 1)

	replies := serveLines(t, s, answerWhenBothParked, realInitialize, realToolsCall, second)

	if len(replies) != 3 {
		t.Fatalf("want 3 replies, got %d", len(replies))
	}
	for _, r := range replies[1:] {
		var v verdict
		if len(r.Result.Content) == 0 {
			t.Fatalf("a gate reply carried no verdict: %+v", r)
		}
		if err := json.Unmarshal([]byte(r.Result.Content[0].Text), &v); err != nil {
			t.Fatalf("unreadable verdict: %v", err)
		}
		if v.Behavior != "allow" {
			t.Errorf("both gates should have been answered together, got %q (%s)", v.Behavior, v.Message)
		}
	}
}

func answerWhenBothParked(s *Store) {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		pending, err := s.List(ApprovalFilter{ActiveOnly: true})
		if err != nil {
			return
		}
		if len(pending) == 2 {
			for _, p := range pending {
				_, _ = s.Decide(p.ID, ActionAllow, "")
			}
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func answerWith(action Action, reason string) func(*Store) {
	return func(s *Store) {
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			pending, err := s.List(ApprovalFilter{ActiveOnly: true})
			if err == nil && len(pending) == 1 {
				_, _ = s.Decide(pending[0].ID, action, reason)
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
	}
}

func lastVerdict(t *testing.T, replies []rpcReply) verdict {
	t.Helper()
	if len(replies) == 0 {
		t.Fatal("the bridge answered nothing")
	}
	last := replies[len(replies)-1]
	if len(last.Result.Content) == 0 {
		t.Fatalf("the reply carried no content block: %+v", last)
	}
	var v verdict
	if err := json.Unmarshal([]byte(last.Result.Content[0].Text), &v); err != nil {
		t.Fatalf("the verdict is not JSON (%v): %q", err, last.Result.Content[0].Text)
	}
	return v
}
