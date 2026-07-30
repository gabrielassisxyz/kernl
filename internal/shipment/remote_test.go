package shipment

import "testing"

func TestNormalizeRemote(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"scp-like ssh", "git@github.com:gabrielassisxyz/archeion.git", "github.com/gabrielassisxyz/archeion"},
		{"scp-like without suffix", "git@github.com:gabrielassisxyz/archeion", "github.com/gabrielassisxyz/archeion"},
		{"https", "https://github.com/gabrielassisxyz/archeion.git", "github.com/gabrielassisxyz/archeion"},
		{"https with credentials", "https://token@github.com/gabrielassisxyz/archeion", "github.com/gabrielassisxyz/archeion"},
		{"ssh url", "ssh://git@github.com/gabrielassisxyz/archeion.git", "github.com/gabrielassisxyz/archeion"},
		{"host case is folded", "git@GitHub.com:Owner/Repo.git", "github.com/Owner/Repo"},
		{"trailing slash", "https://github.com/owner/repo/", "github.com/owner/repo"},
		{"already normalized", "github.com/owner/repo", "github.com/owner/repo"},
		{"local path", "/home/user/repositories/archeion", "/home/user/repositories/archeion"},
		{"file url", "file:///home/user/repositories/archeion", "/home/user/repositories/archeion"},
		{"empty", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeRemote(tc.in); got != tc.want {
				t.Fatalf("NormalizeRemote(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// The path is compared case-sensitively on purpose: GitHub treats owner and
// repository names case-insensitively for lookup but a mismatch here means the
// operator wrote a different name than the one configured, which is worth a
// loud refusal rather than a silent pass.
func TestNormalizeRemoteKeepsPathCase(t *testing.T) {
	if got := NormalizeRemote("git@github.com:Owner/Repo.git"); got != "github.com/Owner/Repo" {
		t.Fatalf("path case folded: %q", got)
	}
}

func TestCheckRemoteAllowed(t *testing.T) {
	allowed := []string{"git@github.com:gabrielassisxyz/archeion.git", "https://github.com/gabrielassisxyz/daytrace"}

	t.Run("matches regardless of the form it was written in", func(t *testing.T) {
		for _, remote := range []string{
			"https://github.com/gabrielassisxyz/archeion.git",
			"git@github.com:gabrielassisxyz/archeion.git",
			"ssh://git@github.com/gabrielassisxyz/daytrace",
		} {
			if err := CheckRemoteAllowed(remote, allowed); err != nil {
				t.Fatalf("remote %q rejected: %v", remote, err)
			}
		}
	})

	t.Run("rejects a remote outside the list", func(t *testing.T) {
		err := CheckRemoteAllowed("git@github.com:someone-else/archeion.git", allowed)
		if err == nil {
			t.Fatal("expected refusal for an unlisted remote")
		}
		assertDispatchFailure(t, err)
	})

	// The empty allow-list is the default, and it denies: a repository that has
	// not declared where it may publish does not publish. The live run that
	// produced this package pushed to an upstream nobody had named.
	t.Run("empty allow-list denies", func(t *testing.T) {
		err := CheckRemoteAllowed("git@github.com:gabrielassisxyz/archeion.git", nil)
		if err == nil {
			t.Fatal("expected refusal when no allowed remote is configured")
		}
		assertDispatchFailure(t, err)
	})

	t.Run("empty remote is refused, never treated as a wildcard", func(t *testing.T) {
		if err := CheckRemoteAllowed("", allowed); err == nil {
			t.Fatal("expected refusal for an empty remote URL")
		}
	})
}

func TestCheckPullRequestAllowed(t *testing.T) {
	allowed := []string{"git@github.com:gabrielassisxyz/archeion.git"}

	t.Run("accepts a PR on an allowed repository", func(t *testing.T) {
		if err := CheckPullRequestAllowed("https://github.com/gabrielassisxyz/archeion/pull/40", allowed); err != nil {
			t.Fatalf("allowed PR rejected: %v", err)
		}
	})

	t.Run("rejects a PR opened somewhere else", func(t *testing.T) {
		err := CheckPullRequestAllowed("https://github.com/someone-else/archeion/pull/1", allowed)
		if err == nil {
			t.Fatal("expected refusal for a PR outside the allow-list")
		}
		assertDispatchFailure(t, err)
	})

	t.Run("rejects a URL it cannot parse into a repository", func(t *testing.T) {
		if err := CheckPullRequestAllowed("not a url", allowed); err == nil {
			t.Fatal("expected refusal for an unparseable PR URL")
		}
	})
}

func assertDispatchFailure(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := err.Error(); len(got) < 22 || got[:22] != "KERNL DISPATCH FAILURE" {
		t.Fatalf("error lacks the greppable marker: %q", got)
	}
}
