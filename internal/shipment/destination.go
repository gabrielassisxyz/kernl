package shipment

import (
	"fmt"
	"os/exec"
	"strings"
)

// DefaultRemoteName is used when a repository does not name one. It is only a
// name to resolve: what shipment validates is the URL it resolves to, which is
// precisely what "push to origin" failed to pin down.
const DefaultRemoteName = "origin"

// Destination is the verified answer to "where does this run publish?".
type Destination struct {
	RemoteName string
	RemoteURL  string
	// RepoSlug is the "host/owner/repo" gh needs as --repo. Empty for a local
	// path remote, which cannot host a pull request.
	RepoSlug string
}

// RemoteLookup reads a remote's URL out of a repository. It exists so the
// policy can be tested without a git process; production passes GitRemoteURL.
type RemoteLookup func(repoPath, remoteName string) (string, error)

// ResolveDestination reads the configured remote out of the repository and
// checks it against the allow-list, before any agent is spawned. It returns a
// loud error rather than a fallback: a shipment stage with an unverifiable
// destination must not run at all.
func ResolveDestination(repoPath, remoteName string, allowed []string, lookup RemoteLookup) (Destination, error) {
	if remoteName == "" {
		remoteName = DefaultRemoteName
	}
	if lookup == nil {
		lookup = GitRemoteURL
	}
	url, err := lookup(repoPath, remoteName)
	if err != nil {
		return Destination{}, err
	}
	if err := CheckRemoteAllowed(url, allowed); err != nil {
		return Destination{}, err
	}
	slug := RepoSlug(url)
	if slug == "" {
		return Destination{}, fmt.Errorf(
			"KERNL DISPATCH FAILURE: remote %q (%s) is not a repository a pull request can be opened on - "+
				"Fix: point shipment at a hosted remote, or run with --dry-run to stop before shipment",
			remoteName, url,
		)
	}
	return Destination{RemoteName: remoteName, RemoteURL: url, RepoSlug: slug}, nil
}

// GitRemoteURL is the production RemoteLookup: a thin shell around
// git remote get-url.
func GitRemoteURL(repoPath, remoteName string) (string, error) {
	return remoteURL(repoPath, remoteName)
}

func remoteURL(repoPath, remoteName string) (string, error) {
	cmd := exec.Command("git", "-C", repoPath, "remote", "get-url", remoteName)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf(
			"KERNL DISPATCH FAILURE: cannot resolve remote %q in %s: %w - "+
				"Fix: add the remote to the repository, or name an existing one in registry.repos[].shipment.remote",
			remoteName, repoPath, err,
		)
	}
	url := strings.TrimSpace(string(out))
	if url == "" {
		return "", fmt.Errorf(
			"KERNL DISPATCH FAILURE: remote %q in %s resolves to an empty URL - Fix: repair the remote with git remote set-url",
			remoteName, repoPath,
		)
	}
	return url, nil
}
