// Package shipment holds the containment policy for the one orchestrator stage
// that acts outside the machine.
//
// Every other stage is bounded by its worktree; shipment pushes and opens pull
// requests. A run against a scratch clone whose origin was a local path still
// reached the public upstream through it, because the prompt said "push to
// origin" and nothing checked what origin resolved to. So the destination is
// declared by the operator and verified by kernl, before dispatch and again
// after, rather than resolved by the agent at runtime.
package shipment

import (
	"fmt"
	"net/url"
	"strings"
)

// NormalizeRemote reduces a git remote to a comparable identity: "host/path"
// for network remotes, an absolute path for local ones. The same repository is
// spelled at least four ways (scp-like ssh, ssh://, https://, with or without a
// .git suffix or embedded credentials), and comparing raw strings would let a
// remote pass or fail depending on which spelling the operator happened to use.
//
// Returns "" for input it cannot reduce, which callers must treat as a refusal
// and never as a wildcard.
func NormalizeRemote(remote string) string {
	remote = strings.TrimSpace(remote)
	if remote == "" {
		return ""
	}

	if rest, ok := strings.CutPrefix(remote, "file://"); ok {
		return strings.TrimRight(rest, "/")
	}
	if strings.HasPrefix(remote, "/") || strings.HasPrefix(remote, ".") || strings.HasPrefix(remote, "~") {
		return strings.TrimRight(remote, "/")
	}

	if host, path, ok := parseSCPLike(remote); ok {
		return joinRemote(host, path)
	}

	// The already-normalized "host/owner/repo" form: what an operator writes by
	// hand in the allow-list, and what this function itself returns. Accepting
	// it keeps NormalizeRemote idempotent, which the allow-list comparison and
	// the pull-request check both rely on.
	if !strings.Contains(remote, "://") {
		if host, path, ok := strings.Cut(remote, "/"); ok && strings.Contains(host, ".") {
			return joinRemote(host, path)
		}
	}

	u, err := url.Parse(remote)
	if err != nil || u.Host == "" {
		return ""
	}
	return joinRemote(u.Hostname(), u.Path)
}

// matchesAllowList reports whether an already-normalized identity is listed.
func matchesAllowList(identity string, allowed []string) bool {
	for _, entry := range allowed {
		if NormalizeRemote(entry) == identity {
			return true
		}
	}
	return false
}

// parseSCPLike handles git's scp-like syntax (user@host:path), which url.Parse
// silently mangles into a scheme of "git@github.com".
func parseSCPLike(remote string) (host, path string, ok bool) {
	if strings.Contains(remote, "://") {
		return "", "", false
	}
	at := strings.Index(remote, "@")
	colon := strings.Index(remote, ":")
	if colon <= 0 || colon < at {
		return "", "", false
	}
	return remote[at+1 : colon], remote[colon+1:], true
}

func joinRemote(host, path string) string {
	host = strings.ToLower(strings.TrimSpace(host))
	path = strings.Trim(strings.TrimSpace(path), "/")
	path = strings.TrimSuffix(path, ".git")
	if host == "" || path == "" {
		return ""
	}
	return host + "/" + path
}

// CheckRemoteAllowed refuses unless the remote resolves to one of the allowed
// entries. An empty allow-list denies everything: a repository that has not
// declared where it may publish does not publish.
func CheckRemoteAllowed(remote string, allowed []string) error {
	if len(allowed) == 0 {
		return fmt.Errorf(
			"KERNL DISPATCH FAILURE: shipment has no allowed remote for this repository, so it refuses to push (resolved remote: %q) - "+
				"Fix: add registry.repos[].shipment.allowedRemotes with the remote this repository may publish to, or run with --dry-run to stop before shipment",
			remote,
		)
	}

	got := NormalizeRemote(remote)
	if got == "" {
		return fmt.Errorf(
			"KERNL DISPATCH FAILURE: shipment cannot interpret the remote %q as a repository identity - "+
				"Fix: point the repository at a normal git remote, or list it verbatim in registry.repos[].shipment.allowedRemotes",
			remote,
		)
	}

	if matchesAllowList(got, allowed) {
		return nil
	}
	return fmt.Errorf(
		"KERNL DISPATCH FAILURE: shipment refuses to push to %q (resolved to %q), which is not in the allow-list %v - "+
			"Fix: add it to registry.repos[].shipment.allowedRemotes if publishing there is intended",
		remote, got, allowed,
	)
}

// CheckPullRequestAllowed verifies, after the fact, that the pull request the
// shipment agent reported lives on an allowed repository. The pre-dispatch
// check removes the ambiguity; this one catches the agent routing around it,
// which is exactly what happened on the run that motivated this package.
func CheckPullRequestAllowed(prURL string, allowed []string) error {
	repo := repoFromPullRequestURL(prURL)
	if repo == "" {
		return fmt.Errorf(
			"KERNL DISPATCH FAILURE: shipment reported pr_url %q, which does not parse as a pull request URL - "+
				"Fix: the shipment stage must write a full https://<host>/<owner>/<repo>/pull/<n> URL",
			prURL,
		)
	}
	if matchesAllowList(repo, allowed) {
		return nil
	}
	return fmt.Errorf(
		"KERNL DISPATCH FAILURE: shipment opened pull request %s on %q, which is not in the allow-list %v - "+
			"Fix: the push destination and the pull request must both be listed in registry.repos[].shipment.allowedRemotes; "+
			"a mismatch here means the agent published somewhere the operator never named",
		prURL, repo, allowed,
	)
}

// repoFromPullRequestURL extracts "host/owner/repo" from a pull request URL,
// tolerating GitHub's /pull/ and GitLab's /-/merge_requests/ shapes.
func repoFromPullRequestURL(prURL string) string {
	u, err := url.Parse(strings.TrimSpace(prURL))
	if err != nil || u.Host == "" {
		return ""
	}
	segments := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(segments) < 3 {
		return ""
	}
	return joinRemote(u.Hostname(), segments[0]+"/"+segments[1])
}
