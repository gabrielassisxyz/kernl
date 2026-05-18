package workflow_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/workflow"
)

type fakeBdRunner struct {
	customs []string
	calls   []string
	failOn  string
}

func (f *fakeBdRunner) GetCustomStatuses() ([]string, error) {
	f.calls = append(f.calls, "get")
	if f.failOn == "get" {
		return nil, errors.New("boom")
	}
	return append([]string(nil), f.customs...), nil
}

func (f *fakeBdRunner) SetCustomStatuses(list []string) error {
	f.calls = append(f.calls, "set:"+strings.Join(list, ","))
	if f.failOn == "set" {
		return errors.New("boom")
	}
	f.customs = append([]string(nil), list...)
	return nil
}

func TestEnsureCustomStatuses_FreshRegistersBoth(t *testing.T) {
	dir := t.TempDir()
	r := &fakeBdRunner{}
	if err := workflow.EnsureCustomStatuses(dir, r); err != nil {
		t.Fatal(err)
	}
	want := strings.Join(workflow.KernlCustomStatuses, ",")
	found := false
	for _, c := range r.calls {
		if c == "set:"+want {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected set call with %q, got %+v", want, r.calls)
	}
}

func TestEnsureCustomStatuses_AlreadyPresent_NoOp(t *testing.T) {
	dir := t.TempDir()
	r := &fakeBdRunner{customs: workflow.KernlCustomStatuses}
	if err := workflow.EnsureCustomStatuses(dir, r); err != nil {
		t.Fatal(err)
	}
	for _, c := range r.calls {
		if strings.HasPrefix(c, "set:") {
			t.Fatalf("expected no set call, got %q", c)
		}
	}
}

func TestEnsureCustomStatuses_ForeignCustomsPreserved(t *testing.T) {
	dir := t.TempDir()
	r := &fakeBdRunner{customs: []string{"awaiting_qa", "needs_design"}}
	if err := workflow.EnsureCustomStatuses(dir, r); err != nil {
		t.Fatal(err)
	}
	var lastSet string
	for _, c := range r.calls {
		if strings.HasPrefix(c, "set:") {
			lastSet = c
		}
	}
	for _, expected := range []string{"awaiting_qa", "needs_design", "awaiting_integration", "awaiting_pr_review"} {
		if !strings.Contains(lastSet, expected) {
			t.Fatalf("expected %q in %q", expected, lastSet)
		}
	}
}

func TestEnsureCustomStatuses_BdFailure_Propagates(t *testing.T) {
	dir := t.TempDir()
	r := &fakeBdRunner{failOn: "set"}
	if err := workflow.EnsureCustomStatuses(dir, r); err == nil {
		t.Fatal("expected error to propagate")
	}
}

func TestEnsureCustomStatuses_CachedAfterFirstCall(t *testing.T) {
	workflow.ResetEnsureCache()
	dir := t.TempDir()
	r := &fakeBdRunner{}
	_ = workflow.EnsureCustomStatuses(dir, r)
	callsAfterFirst := len(r.calls)
	_ = workflow.EnsureCustomStatuses(dir, r)
	if len(r.calls) != callsAfterFirst {
		t.Fatalf("expected cache hit, second call added %d more calls", len(r.calls)-callsAfterFirst)
	}
}
