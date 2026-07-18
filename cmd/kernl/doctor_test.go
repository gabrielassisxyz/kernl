package main

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/preflight"
)

var errNotFound = errors.New("executable file not found in $PATH")

func doctorReportFor(t *testing.T, deps preflight.Deps) doctorReport {
	t.Helper()
	return newDoctorReport(preflight.Run(deps))
}

func TestDoctorReportRecommendsFirstRequiredFix(t *testing.T) {
	rep := doctorReportFor(t, preflight.Deps{
		LookPath:     func(string) (string, error) { return "", errNotFound },
		ConfigPath:   "does-not-exist.yaml",
		GoVersion:    "go1.26",
		Orchestrator: true,
	})
	if rep.OK {
		t.Fatal("report with failing required checks must have ok=false")
	}
	if rep.RecommendedAction == "" || rep.RecommendedAction == "all required checks passed" {
		t.Fatalf("failing report must recommend the first required fix, got %q", rep.RecommendedAction)
	}
}

func TestDoctorReportMarshalsCamelCase(t *testing.T) {
	rep := doctorReportFor(t, preflight.Deps{
		LookPath:     func(string) (string, error) { return "", errNotFound },
		ConfigPath:   "does-not-exist.yaml",
		GoVersion:    "go1.26",
		Orchestrator: true,
	})
	b, err := json.Marshal(rep)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"ok", "checks", "recommendedAction"} {
		if _, present := raw[key]; !present {
			t.Errorf("doctor JSON missing camelCase key %q: %s", key, b)
		}
	}
}

func TestDoctorRejectsUnknownFlagWithHint(t *testing.T) {
	err := runDoctor("kernl.yaml", []string{"--jsn"})
	if err == nil || exitCode(err) != 2 {
		t.Fatalf("unknown doctor flag must be usage error, got: %v", err)
	}
}
