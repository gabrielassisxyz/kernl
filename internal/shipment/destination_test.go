package shipment

import (
	"errors"
	"strings"
	"testing"
)

// stubRemoteLookup answers with a fixed URL, recording what it was asked for.
type stubRemoteLookup struct {
	url        string
	err        error
	askedRepo  string
	askedRemot string
}

func (s *stubRemoteLookup) lookup(repoPath, remoteName string) (string, error) {
	s.askedRepo = repoPath
	s.askedRemot = remoteName
	return s.url, s.err
}

func TestResolveDestination(t *testing.T) {
	allowed := []string{"github.com/gabrielassisxyz/archeion"}

	t.Run("resolves and accepts an allowed remote", func(t *testing.T) {
		stub := &stubRemoteLookup{url: "git@github.com:gabrielassisxyz/archeion.git"}
		dest, err := ResolveDestination("/repo", "", allowed, stub.lookup)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dest.RemoteURL != stub.url {
			t.Fatalf("RemoteURL = %q, want %q", dest.RemoteURL, stub.url)
		}
		if dest.RemoteName != DefaultRemoteName {
			t.Fatalf("RemoteName = %q, want the default %q", dest.RemoteName, DefaultRemoteName)
		}
		if stub.askedRepo != "/repo" || stub.askedRemot != DefaultRemoteName {
			t.Fatalf("lookup asked for (%q, %q)", stub.askedRepo, stub.askedRemot)
		}
	})

	t.Run("honors a configured remote name", func(t *testing.T) {
		stub := &stubRemoteLookup{url: "git@github.com:gabrielassisxyz/archeion.git"}
		if _, err := ResolveDestination("/repo", "publish", allowed, stub.lookup); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stub.askedRemot != "publish" {
			t.Fatalf("looked up remote %q, want %q", stub.askedRemot, "publish")
		}
	})

	// The failure this whole package exists for: the scratch clone's origin was
	// a local path, the agent reached the public upstream through it, and
	// nothing objected. Here the local origin is simply not allowed.
	t.Run("refuses a remote outside the allow-list", func(t *testing.T) {
		stub := &stubRemoteLookup{url: "/home/user/repositories/archeion"}
		if _, err := ResolveDestination("/repo", "", allowed, stub.lookup); err == nil {
			t.Fatal("expected refusal for a remote outside the allow-list")
		}
	})

	t.Run("refuses when no allow-list is configured", func(t *testing.T) {
		stub := &stubRemoteLookup{url: "git@github.com:gabrielassisxyz/archeion.git"}
		_, err := ResolveDestination("/repo", "", nil, stub.lookup)
		if err == nil {
			t.Fatal("expected refusal when allowedRemotes is empty")
		}
		if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
			t.Fatalf("error lacks the greppable marker: %v", err)
		}
	})

	t.Run("surfaces a lookup failure instead of falling back", func(t *testing.T) {
		stub := &stubRemoteLookup{err: errors.New("no such remote")}
		if _, err := ResolveDestination("/repo", "", allowed, stub.lookup); err == nil {
			t.Fatal("expected the lookup error to propagate")
		}
	})
}
