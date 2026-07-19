package api

import (
	"encoding/json"
	"net/http"
)

func RegisterAppRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/app-update", appUpdateHandler)
}

// appUpdateHandler is a PLACEHOLDER: kernl checks no release feed, so it
// reports "unknown" rather than an answer it never computed. It previously
// returned "up_to_date" unconditionally, which is the one failure mode that
// actually costs the user something — a false all-clear can talk someone out of
// a security update.
//
// Implementing this for real needs product decisions that have not been made:
// whether the binary may make outbound calls at all, against which feed (the
// GitHub releases API for gabrielassisxyz/kernl), how often, and where the
// result is cached so the UI does not hit the network on every poll. It also
// needs the build's own version, which today lives in package main
// (cmd/kernl.Version, set by goreleaser ldflags) and is not plumbed into the
// API layer — hence no "currentVersion" field here yet.
func appUpdateHandler(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	// checked=false is the field a client should branch on: it separates "we
	// looked and found nothing newer" from "nobody looked".
	json.NewEncoder(w).Encode(map[string]any{
		"status":  "unknown",
		"checked": false,
		"detail":  "update checking is not implemented; kernl contacts no release feed",
	})
}
