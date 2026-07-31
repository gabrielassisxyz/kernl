package review

import "testing"

func TestParse(t *testing.T) {
	if k, ok := Parse("fixup"); !ok || k != KindFixup {
		t.Errorf("Parse(fixup) = %v, %v", k, ok)
	}
	if k, ok := Parse("decision"); !ok || k != KindDecision {
		t.Errorf("Parse(decision) = %v, %v", k, ok)
	}
	for _, bad := range []string{"", "maybe", "FIXUP", "decisions"} {
		if _, ok := Parse(bad); ok {
			t.Errorf("Parse(%q) must not be ok - the vocabulary is fixed and case-sensitive by design (the caller lowercases first)", bad)
		}
	}
}

func TestAll(t *testing.T) {
	all := All()
	if len(all) != 2 {
		t.Fatalf("All() = %v, want exactly the two kinds", all)
	}
	if all[0] != KindFixup || all[1] != KindDecision {
		t.Errorf("All() = %v, want [fixup decision]", all)
	}
}
