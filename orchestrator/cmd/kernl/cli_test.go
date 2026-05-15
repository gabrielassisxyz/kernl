package main

import (
	"strings"
	"testing"
)

func TestDispatchUnknownSubcommandFailsLoud(t *testing.T) {
	err := Dispatch([]string{"frobnicate"})
	if err == nil || !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Fatalf("expected loud failure for unknown subcommand, got %v", err)
	}
}

func TestDispatchDoctorRunsPreflight(t *testing.T) {
	var ran bool
	doctorFn = func(configPath string) error { ran = true; return nil }
	t.Cleanup(func() { doctorFn = runDoctor })
	if err := Dispatch([]string{"doctor"}); err != nil || !ran {
		t.Fatalf("doctor not dispatched: ran=%v err=%v", ran, err)
	}
}

func TestDispatchServeRunsServer(t *testing.T) {
	var ran bool
	serveFn = func(configPath string) error { ran = true; return nil }
	t.Cleanup(func() { serveFn = runServe })
	if err := Dispatch([]string{"serve"}); err != nil || !ran {
		t.Fatalf("serve not dispatched: ran=%v err=%v", ran, err)
	}
}

func TestDispatchHelpPrintsSubcommands(t *testing.T) {
	var ran bool
	helpFn = func() error { ran = true; return nil }
	t.Cleanup(func() { helpFn = printHelp })
	if err := Dispatch([]string{"--help"}); err != nil || !ran {
		t.Fatalf("help not dispatched: ran=%v err=%v", ran, err)
	}
}

func TestDispatchEpicAndBeadReturnError(t *testing.T) {
	// epic and bead are not yet implemented — they should return an error
	for _, cmd := range []string{"epic", "bead"} {
		err := Dispatch([]string{cmd})
		if err == nil {
			t.Fatalf("expected stubbed error for %s, got nil", cmd)
		}
	}
}
