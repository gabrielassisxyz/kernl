package backend

import "testing"

// TestDecisionRecordSectionBodiesAgreesWithMissingSections pins
// missingDecisionRecordSections and DecisionRecordSectionBodies to the same,
// independently-known-correct set of recognized sections for a range of
// documents, including the bypass fixtures from
// state_machine_decision_record_parser_test.go (fenced headings, commented
// headings, setext headings, a UTF-8 BOM, horizontal-rule-only bodies). Each
// case is checked two ways: against an explicit "want missing" set (so a bug
// shared by both functions - not just a divergence between them - is still
// caught), and for internal agreement between the two (a body present with
// real content must never also be reported missing, and vice versa). The two
// functions share one parsing pass (DecisionRecordSectionBodies is exported
// and missingDecisionRecordSections is defined on top of it), so this test
// would fail the moment anyone reintroduces a second, independent
// implementation of "what counts as a section" for either side.
func TestDecisionRecordSectionBodiesAgreesWithMissingSections(t *testing.T) {
	wellFormed := "## Decision\n\nUse Rust.\n\n" +
		"## Options Considered\n\nRust vs Go vs C++.\n\n" +
		"## Trade-offs\n\nRust has a steeper learning curve.\n\n" +
		"## Rationale\n\nMemory safety without a GC.\n"

	fenced := "```\n## Decision\nplaceholder\n## Options Considered\nplaceholder\n" +
		"## Trade-offs\nplaceholder\n## Rationale\nplaceholder\n```\n"

	commented := "<!--\n## Decision\nplaceholder\n## Options Considered\nplaceholder\n" +
		"## Trade-offs\nplaceholder\n## Rationale\nplaceholder\n-->\n"

	setext := "Decision\n--------\n\nUse Rust.\n\n" +
		"Options Considered\n-------------------\n\nRust vs Go.\n\n" +
		"Trade-offs\n----------\n\nSteeper curve.\n\n" +
		"Rationale\n---------\n\nMemory safety.\n"

	partial := "## Decision\n\nUse Rust.\n\n## Options Considered\n\nRust vs Go.\n"

	bom := "\uFEFF" + wellFormed

	// Every section body is nothing but a horizontal rule - presentation,
	// not content.
	horizontalRuleOnly := "## Decision\n---\n" +
		"## Options Considered\n---\n" +
		"## Trade-offs\n---\n" +
		"## Rationale\n---\n"

	cases := []struct {
		name        string
		content     string
		wantMissing []string
	}{
		{"well-formed", wellFormed, nil},
		{"fenced-bypass", fenced, allFourKeys},
		{"commented-bypass", commented, allFourKeys},
		{"setext", setext, nil},
		{"partial", partial, []string{"trade_offs", "rationale"}},
		{"bom", bom, nil},
		{"horizontal-rule-only", horizontalRuleOnly, allFourKeys},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assertMissing(t, c.content, c.wantMissing)

			missing := missingDecisionRecordSections(c.content)
			missingSet := make(map[string]bool, len(missing))
			for _, key := range missing {
				missingSet[key] = true
			}

			bodies := DecisionRecordSectionBodies(c.content)

			for _, s := range decisionRecordSections {
				body, present := bodies[s.key]
				hasRealBody := present && body != ""
				isMissing := missingSet[s.key]
				if hasRealBody == isMissing {
					t.Errorf("section %q: DecisionRecordSectionBodies hasRealBody=%v but missingDecisionRecordSections says missing=%v - the two disagree", s.key, hasRealBody, isMissing)
				}
			}
		})
	}
}
