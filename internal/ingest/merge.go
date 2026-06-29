package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/chat"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/graph/nodes"
)

// MergePlan is the proposal for an Update merge: the target note to merge into
// and the additive hunks for the user to accept or reject. An empty TargetNoteID
// means no confident target was found and the caller should fall back to Create
// Page.
type MergePlan struct {
	TargetNoteID string      `json:"targetNoteId"`
	TargetTitle  string      `json:"targetTitle"`
	CurrentBody  string      `json:"currentBody"`
	Hunks        []MergeHunk `json:"hunks"`
}

const mergeSystemPrompt = `You merge new content into an existing knowledge-base note. Identify the information in the NEW CONTENT that is not already covered by the EXISTING NOTE and should be added to it.

Return ONLY a JSON array of strings (no prose, no markdown fences). Each string is one self-contained block of text to append to the note. Omit anything already present in the existing note. If the new content adds nothing, return an empty array [].`

// PlanMerge resolves the target note for an Update review and asks the LLM for
// the additive hunks to merge in. The returned plan drives the DiffSuggest
// accept/reject UI; the accepted subset is fed back through ResolveReview.
func PlanMerge(ctx context.Context, g *graph.Graph, llm chat.LLMClient, reviewID string) (*MergePlan, error) {
	review, err := readReview(ctx, g, reviewID)
	if err != nil {
		return nil, err
	}
	return PlanMergeFor(ctx, g, llm, review.Payload, review.SourceNodeID)
}

// PlanMergeFor resolves the best note to merge payload into (excluding
// sourceNodeID) and asks the LLM for the additive hunks. An empty TargetNoteID
// in the returned plan means no confident target — the caller should fall back
// to creating a page/note. Shared by the ingest queue and the inbox.
func PlanMergeFor(ctx context.Context, g *graph.Graph, llm chat.LLMClient, payload, sourceNodeID string) (*MergePlan, error) {
	targetID, err := ResolveMergeTargetFor(ctx, g, payload, sourceNodeID)
	if err != nil {
		return nil, err
	}
	if targetID == "" {
		return &MergePlan{}, nil
	}

	var target *nodes.Note
	if err := g.DoRead(ctx, func(tx *graph.ReadTx) error {
		var e error
		target, e = nodes.GetNote(ctx, tx, targetID)
		return e
	}); err != nil {
		return nil, err
	}

	hunks, err := proposeMergeHunks(ctx, llm, target.Body, payload)
	if err != nil {
		return nil, err
	}

	return &MergePlan{
		TargetNoteID: target.ID,
		TargetTitle:  target.Title,
		CurrentBody:  target.Body,
		Hunks:        hunks,
	}, nil
}

// proposeMergeHunks asks the LLM for additive content blocks. Unparseable output
// is treated as "nothing to merge" rather than an error, so a malformed response
// never blocks the review.
func proposeMergeHunks(ctx context.Context, llm chat.LLMClient, existing, incoming string) ([]MergeHunk, error) {
	prompt := fmt.Sprintf("%s\n\nEXISTING NOTE:\n%s\n\nNEW CONTENT:\n%s", mergeSystemPrompt, existing, incoming)

	resp, err := llm.Chat(ctx, []chat.Message{{Role: "user", Content: prompt}}, nil)
	if err != nil {
		return nil, fmt.Errorf("llm merge: %w", err)
	}

	raw := extractJSONArray(resp.Content)
	if raw == "" {
		return []MergeHunk{}, nil
	}

	var blocks []string
	if err := json.Unmarshal([]byte(raw), &blocks); err != nil {
		return []MergeHunk{}, nil
	}

	hunks := make([]MergeHunk, 0, len(blocks))
	for i, b := range blocks {
		b = strings.TrimSpace(b)
		if b == "" {
			continue
		}
		hunks = append(hunks, MergeHunk{ID: strconv.Itoa(i), Content: b})
	}
	return hunks, nil
}
