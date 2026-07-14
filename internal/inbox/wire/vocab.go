package wire

import "strings"

// The vocabulary that defines what a capture action MEANS — the prose half of
// the contract whose shape lives in action.go.
//
// It sits here for the same reason the shape does: it now has three consumers.
// The classifier's single-capture and batch prompts embed it (internal/inbox),
// and so does the routing mode of the chat engine (internal/chat), which cannot
// import internal/inbox without a cycle. Two copies of "what is a project" is
// how the DA and the classifier start quietly disagreeing about the same
// capture.

// TargetVocabulary is the split-then-classify rule the model applies to a
// capture. It is the definition of each target and, more importantly, of when a
// capture must be split into several.
const TargetVocabulary = `A capture is often MORE THAN ONE THING. Work in two steps: FIRST split the capture into distinct items, THEN pick a target for each one. Never fold two items into one action.

Targets:
- "project": TWO tests, both required. (1) ONE outcome: you can name the state of the world when it is done. (2) MORE THAN ONE STEP to get there. A COLLECTION — a list, a backlog, a dump of items — is NOT a project, however long it is: it has no single outcome, so it is N items, each classified on its own.
- "task": one concrete action, done in one sitting, indivisible. A question is a task (answering it is the action; the note is what gets written once it is answered).
- "update": the capture extends or revises a topic that almost certainly already has its own note. Use it alone, never combined with other actions.
- "note": durable knowledge, a reflection, or an insight worth preserving.
- "bookmark": a URL or external reference to save.
- "discard": this fragment is noise. Discarding one action does not discard the capture.

Splitting rules:
- A message holding several items (two unrelated ideas typed in one go) yields ONE ACTION PER ITEM.
- An agenda list ("tomorrow:", "today:", "plan:", a list of errands) is a LIST OF SEPARATE ITEMS: one action per line. It is NOT a project — a list is not an outcome. Only group the lines into a project when EVERY line serves one shared outcome; if even one line belongs elsewhere, they are separate actions.
- Judge by the items, not by how the capture labels itself. A capture calling itself a "plan" or a "project" is still a list of separate items when its lines do not share one outcome.
- A reflection that also implies an action is a "note" AND a "task". A sentence about how you think, feel, or work — an insight, a realization, a self-observation — is a note, even when it sits in the same message as an action.
- A verb-initial bookmark ("Reread: <url>", "Watch: <url>") is a "bookmark" AND a "task".
- Do not shrink a project into a task because it sounds small: "more than one step" is the floor, not "sounds ambitious". Do not classify an actionable idea as a note because it is phrased informally.
- Never invent a project whose initial_tasks only restate the capture ("define X", "do X", "adjust X"). One action, split into synonyms of itself, is still ONE TASK.`

// ActionFieldRules describes the per-action fields. The title rule is the one
// that makes a long paste reviewable: the user reads titles, not bodies.
const ActionFieldRules = `Field rules:
- title: ALWAYS write one. Short, imperative, human. Never the truncated body. The user reads titles, not bodies: a title has to say what the node IS without the capture next to it.
- body: the fragment of the capture this action owns, plus whatever surrounding context makes it stand on its own weeks later. Omit when the action owns the whole capture.
- project_id: an existing project id from the list above, for a task that belongs to it.
- project_title/project_description/initial_tasks: only for "project"; 3-6 short initial_tasks.
- due_date: "YYYY-MM-DD", on a "task" only. Set it ONLY when the capture itself states a deadline ("amanhã", "tomorrow", "até sexta", "by friday", "this weekend", an explicit date). Take the value from the date anchors above — do not compute it, and do not use the real current date. No deadline stated → null. NEVER invent one: a task with no due date is normal.
- tags: 0-3 tags naming what the node is ABOUT, so it can be found later without a search. CLOSED LIST — use ONLY these, exactly as spelled: ` + TagVocabulary + `
  Never coin a new tag, however well it seems to fit. Never tag a node with what it already IS ("capture", "task", "note", "project"): the node's type already says that, and such a tag filters nothing. When no tag on the list applies, send none — an empty list is a correct answer.`

// TagVocabulary is the closed set of tags a capture action may carry. It is an
// enum, not a hint: an open vocabulary fragments into to-read / toread /
// reading / read-later, and the model reaches for restatements of the node's own
// type ("capture", "task") that filter nothing. A tag earns its place here when
// the user finds themselves wanting to filter by it — the list grows by decision,
// not by generation.
//
// "behavior" is the one that had to be named: a capture like "use the recorder
// with headphones, not with the phone in my hand" is not a one-off errand — it
// is a way of working the user wants to adopt. Without a word for it, it was
// filed as a bare task and lost the thing that made it worth keeping.
const TagVocabulary = `"behavior" (a habit or way of working to adopt or drop — a recurring practice, not a one-off errand), "to-read" (something to read or watch later), "idea", "question", "reflection", "bug", "research", "purchase", "health", "admin"`

// KnownTags is TagVocabulary as data. A tag the model coined anyway is dropped
// rather than written: the prompt asks, this enforces.
var KnownTags = []string{
	"behavior", "to-read", "idea", "question", "reflection",
	"bug", "research", "purchase", "health", "admin",
}

// FilterTags normalises tags and keeps only those on the closed list. It is the
// backstop for the prompt: an LLM told "never coin a tag" still occasionally
// does. It normalises first because a model that returns "To-Read" means the
// tag on the list, and dropping it as unknown loses real information silently —
// the filter exists to reject invention, not spelling.
func FilterTags(tags []string) []string {
	var out []string
	for _, t := range tags {
		t = strings.ToLower(strings.TrimSpace(t))
		for _, known := range KnownTags {
			if t == known {
				out = append(out, t)
				break
			}
		}
	}
	return out
}

// TargetGlossary is TargetVocabulary compressed to what a CONVERSATION needs.
//
// The long form above is a classifier prompt: it recites splitting rules to a
// model whose whole job is to emit JSON. Handed to a tool-calling chat it did
// the opposite of its job — at ~3.4k characters it buried the one instruction
// that mattered ("call the tool"), and three different models answered in prose
// instead of calling it, about two times in three. The same models called the
// tool 3/3 with a short prompt. Length is not free: it is attention spent.
const TargetGlossary = `The targets: "note" (durable knowledge, a reflection, an insight), "task" (one concrete action, done in one sitting), "project" (ONE outcome reached in SEVERAL steps — a list of unrelated items is not a project), "bookmark" (a URL or external reference), "update" (folds into a note that already exists; it must stand ALONE, never beside other nodes), "discard" (noise).

One capture is routinely SEVERAL nodes: split it into items first, then type each one. A reflection that also implies an action is a note AND a task.`

// Targets is every target a capture action may carry. An unknown target is a
// model hallucination and is rejected rather than written.
var Targets = []string{"note", "update", "bookmark", "task", "project", "discard"}

// ValidTarget reports whether t is a target the graph knows how to write.
func ValidTarget(t string) bool {
	for _, known := range Targets {
		if t == known {
			return true
		}
	}
	return false
}
