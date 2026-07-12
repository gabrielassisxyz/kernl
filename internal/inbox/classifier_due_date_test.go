package inbox

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// The whole point of the phase: "tomorrow" is relative to the day the message
// was WRITTEN. The backlog is months old, so resolving it against the real today
// would date every deadline in it wrong.
func TestCaptureReferenceTimeIsWhenTheMessageWasWritten(t *testing.T) {
	// The fixture's 21:08 "amanhã:" message, as parseWhatsAppBatch hands it over.
	capture := &nodes.Capture{
		Body:           "amanhã:",
		BatchTimestamp: "4/1/26 21:08",
		CreatedAt:      time.Date(2026, 7, 12, 18, 0, 0, 0, time.UTC), // pasted today
	}
	ref := captureReferenceTime(capture)
	if got := ref.Format(nodes.DueDateLayout); got != "2026-04-01" {
		t.Fatalf("reference date = %s, want 2026-04-01 (the day the message was sent)", got)
	}

	// No batch header (a quick capture): the graph's own timestamp is the day.
	quick := &nodes.Capture{Body: "call the dentist tomorrow", CreatedAt: time.Date(2026, 7, 12, 18, 0, 0, 0, time.UTC)}
	if got := captureReferenceTime(quick).Format(nodes.DueDateLayout); got != "2026-07-12" {
		t.Fatalf("quick capture reference date = %s, want its creation day 2026-07-12", got)
	}
}

func TestParseBatchTimestamp(t *testing.T) {
	cases := []struct {
		raw  string
		want string // "" = unparseable
	}{
		{"4/1/26 21:08", "2026-04-01"},   // month-first, as WhatsApp exports here
		{"4/1/2026 21:08", "2026-04-01"}, // four-digit year
		{"12/25/26 08:00", "2026-12-25"}, // month-first, unambiguous
		{"13/4/26 10:00", "2026-04-13"},  // 13 cannot be a month: day-first fallback
		{"4/1/26 21:08:33", "2026-04-01"},
		{"", ""},
		{"yesterday", ""},
	}
	for _, tc := range cases {
		got, ok := parseBatchTimestamp(tc.raw)
		if tc.want == "" {
			if ok {
				t.Errorf("parseBatchTimestamp(%q) = %v, want no timestamp", tc.raw, got)
			}
			continue
		}
		if !ok {
			t.Errorf("parseBatchTimestamp(%q) failed, want %s", tc.raw, tc.want)
			continue
		}
		if day := got.Format(nodes.DueDateLayout); day != tc.want {
			t.Errorf("parseBatchTimestamp(%q) = %s, want %s", tc.raw, day, tc.want)
		}
	}
}

// The anchors are computed in Go and handed to the model as a lookup table, so
// the model never does date arithmetic. This is the assertion that pins the
// fixture's expected answer.
func TestDateAnchorsResolveRelativeWordsAgainstTheCapture(t *testing.T) {
	ref := time.Date(2026, 4, 1, 21, 8, 0, 0, time.UTC) // Wednesday
	line := dateAnchorLine(ref)

	for _, want := range []string{
		"written 2026-04-01 (Wednesday)",
		"today=2026-04-01",
		"tomorrow=2026-04-02", // the "amanhã:" answer
		"friday=2026-04-03",   // "até sexta"
		"next-week=2026-04-08",
	} {
		if !strings.Contains(line, want) {
			t.Errorf("anchor line is missing %q:\n%s", want, line)
		}
	}
	// The capture's own weekday means a week out, not the same day.
	if !strings.Contains(line, "wednesday=2026-04-08") {
		t.Errorf("a weekday must resolve to its NEXT occurrence:\n%s", line)
	}
}

// The prompt is ours, not the model's, so asserting on it is fair game — and it
// is where the reference date either reaches the model or does not.
func TestClassifyPromptCarriesTheCaptureDateNotToday(t *testing.T) {
	ctx := context.Background()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	var seenPrompt string
	llm := &promptCapturingLLM{
		onPrompt: func(p string) { seenPrompt = p },
		reply:    `{"actions":[{"target":"task","title":"Gather the substack resources","due_date":"2026-04-02"}]}`,
	}
	c := NewClassifier(g, llm, ClassifierOptions{})

	capture := &nodes.Capture{
		Body:           "amanhã: juntar os recursos do substack",
		BatchTimestamp: "4/1/26 21:08",
		CreatedAt:      time.Now(), // pasted today, months later
	}
	got, err := c.classify(ctx, capture, nil)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(seenPrompt, "tomorrow=2026-04-02") {
		t.Errorf("the prompt must resolve tomorrow against the capture's own day:\n%s", seenPrompt)
	}
	if !strings.Contains(seenPrompt, "written 2026-04-01") {
		t.Errorf("the prompt must state the day the capture was written:\n%s", seenPrompt)
	}
	if today := time.Now().Format(nodes.DueDateLayout); strings.Contains(seenPrompt, "today="+today) {
		t.Errorf("the prompt anchors on the real today (%s) instead of the capture's day:\n%s", today, seenPrompt)
	}
	if nodes.FormatDueDate(got[0].DueDate) != "2026-04-02" {
		t.Errorf("proposed due date = %q, want 2026-04-02", nodes.FormatDueDate(got[0].DueDate))
	}
}

// A batch spans days, so each capture carries its own written-on date.
func TestClassifyBatchPromptCarriesEachCaptureDate(t *testing.T) {
	ctx := context.Background()
	g, err := graph.Open(ctx, graph.Config{Path: filepath.Join(t.TempDir(), "graph.db")})
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	var seenPrompt string
	llm := &promptCapturingLLM{onPrompt: func(p string) { seenPrompt = p }, reply: `{"items":[]}`}
	c := NewClassifier(g, llm, ClassifierOptions{})

	group := []*nodes.Capture{
		{ID: "a", Body: "amanhã: revisar o backlog", BatchSequence: 0, BatchTimestamp: "4/1/26 21:08"},
		{ID: "b", Body: "hoje: pagar a conta", BatchSequence: 1, BatchTimestamp: "4/3/26 09:00"},
	}
	if _, err := c.classifyBatch(ctx, group, nil); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"[0] (written 2026-04-01)",
		"[1] (written 2026-04-03)",
		"tomorrow=2026-04-02", // anchors for the first capture's day
		"tomorrow=2026-04-04", // ...and for the second's
	} {
		if !strings.Contains(seenPrompt, want) {
			t.Errorf("batch prompt is missing %q:\n%s", want, seenPrompt)
		}
	}
}

func TestNormalizeActionsKeepsDueDateOnTasksOnly(t *testing.T) {
	got := normalizeActions([]rawAction{
		{Target: "task", Title: "Read the PDFs", DueDate: "2026-04-02"},
		{Target: "note", Title: "An insight", DueDate: "2026-04-02"},
		{Target: "task", Title: "No deadline stated"},
		{Target: "task", Title: "Model wrote prose", DueDate: "amanhã"},
	}, nil)

	if len(got) != 4 {
		t.Fatalf("got %d actions, want 4", len(got))
	}
	if nodes.FormatDueDate(got[0].DueDate) != "2026-04-02" {
		t.Errorf("task due date = %q, want 2026-04-02", nodes.FormatDueDate(got[0].DueDate))
	}
	if got[1].DueDate != nil {
		t.Errorf("a note must not carry a due date: %v", got[1].DueDate)
	}
	if got[2].DueDate != nil {
		t.Errorf("no deadline stated must stay nil: %v", got[2].DueDate)
	}
	// An unreadable date is dropped rather than guessed at — the action survives.
	if got[3].DueDate != nil {
		t.Errorf("an unparseable due date must be dropped: %v", got[3].DueDate)
	}
	if got[3].Target != "task" || got[3].Title != "Model wrote prose" {
		t.Errorf("a bad date must not cost the whole action: %#v", got[3])
	}
}

func TestParseActionsReadsDueDate(t *testing.T) {
	got := parseActions(`{"actions":[
		{"target":"task","title":"Gather the substack resources","due_date":"2026-04-02"},
		{"target":"task","title":"Sort the to-read backlog","due_date":null}
	]}`, nil)

	if len(got) != 2 {
		t.Fatalf("got %d actions, want 2", len(got))
	}
	if nodes.FormatDueDate(got[0].DueDate) != "2026-04-02" {
		t.Errorf("due date = %q, want 2026-04-02", nodes.FormatDueDate(got[0].DueDate))
	}
	if got[1].DueDate != nil {
		t.Errorf("a null due_date must stay nil, got %v", got[1].DueDate)
	}
}
