package vault

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// PolicyParser evaluates .kernl-policies files at the vault root.
//
// The file contains deny/allow rules, one per line:
//
//	PATTERN    — deny (path.Match glob, * does not cross /)
//	!PATTERN   — allow (exception)
//
// Lines are evaluated top to bottom; the last matching rule wins.
// If no rule matches, or the file is missing, access is allowed.
//
// A simple cache avoids re-reading the file on every call:
// reload if mtime changed or a 5-second TTL has elapsed.
type PolicyParser struct {
	vaultRoot    string
	policiesPath string // filepath.Join(vaultRoot, ".kernl-policies")

	mu      sync.Mutex
	modTime time.Time
	rules   []cachedRule
	checked time.Time // last time we checked mtime (for TTL)
}

type cachedRule struct {
	pattern string
	allow   bool // true → allow, false → deny
}

const policyRefreshTTL = 5 * time.Second

// NewPolicyParser returns a parser backed by .kernl-policies at the given
// vault root. vaultRoot must be an absolute path (as provided by config).
func NewPolicyParser(vaultRoot string) *PolicyParser {
	return &PolicyParser{
		vaultRoot:    vaultRoot,
		policiesPath: filepath.Join(vaultRoot, ".kernl-policies"),
	}
}

// CanReadGlobal evaluates the given relative path against the rules in
// .kernl-policies. The path is relative to the vault root (e.g.
// "notes/secret.md"). Leading slashes are stripped before matching.
//
// Returns true if the path is allowed; false if denied by a matching rule.
func (p *PolicyParser) CanReadGlobal(path string) bool {
	path = strings.TrimLeft(path, "/")
	if path == "" {
		return true
	}

	rules, ok := p.loadRules()
	if !ok {
		// file missing → allow all
		return true
	}

	// Evaluate top-to-bottom; last matching rule wins.
	allowed := true // default permissive
	for _, r := range rules {
		if match, _ := filepath.Match(r.pattern, path); match {
			allowed = r.allow
		}
	}
	return allowed
}

// loadRules reads and parses the policies file, using a simple cache.
// Returns (rules, true) on success, or (nil, false) when the file is absent.
func (p *PolicyParser) loadRules() ([]cachedRule, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	info, err := os.Stat(p.policiesPath)
	if err != nil {
		if os.IsNotExist(err) {
			p.rules = nil
			p.modTime = time.Time{}
			p.checked = time.Time{}
			return nil, false
		}
		// other stat errors → treat as missing, allow all
		return nil, false
	}

	// Cache hit: mtime unchanged and within TTL.
	if !p.checked.IsZero() && info.ModTime().Equal(p.modTime) &&
		time.Since(p.checked) < policyRefreshTTL {
		return p.rules, true
	}

	// Reload.
	data, err := os.ReadFile(p.policiesPath)
	if err != nil {
		// file vanished mid-check → allow all
		return nil, false
	}

	p.modTime = info.ModTime()
	p.checked = time.Now()
	p.rules = parseRules(string(data))
	return p.rules, true
}

// parseRules converts file content into a slice of cachedRule.
// Empty lines and lines starting with '#' are ignored.
func parseRules(content string) []cachedRule {
	lines := strings.Split(content, "\n")
	rules := make([]cachedRule, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "!") {
			rules = append(rules, cachedRule{
				pattern: strings.TrimSpace(line[1:]),
				allow:   true,
			})
		} else {
			rules = append(rules, cachedRule{
				pattern: line,
				allow:   false,
			})
		}
	}
	return rules
}
