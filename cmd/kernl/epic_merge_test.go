package main

import (
	"strings"
	"testing"

	"github.com/gabrielassisxyz/kernl/internal/app"
	"github.com/gabrielassisxyz/kernl/internal/config"
)

// runEpicMerge now (re-)drives the epic integration stages; the drive logic is
// covered hermetically by internal/app TestDriveEpic_*. Here we only assert the
// cheap up-front argument/config validation.

func TestEpicMergeRequiresEpicID(t *testing.T) {
	a := &app.App{Config: &config.Config{Registry: config.RegistryConfig{Repos: []config.RepoEntry{{Path: "/test"}}}}}
	err := runEpicMerge(a, nil, func(string) {})
	if err == nil || !strings.Contains(err.Error(), "requires an epic ID") {
		t.Fatalf("want requires-an-epic-id error, got %v", err)
	}
}

func TestEpicMergeRequiresRepo(t *testing.T) {
	a := &app.App{Config: &config.Config{}}
	err := runEpicMerge(a, []string{"e1"}, func(string) {})
	if err == nil || !strings.Contains(err.Error(), "no repos registered") {
		t.Fatalf("want no-repos error, got %v", err)
	}
}
