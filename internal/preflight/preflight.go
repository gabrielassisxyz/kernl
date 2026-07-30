package preflight

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/config"
)

type Check struct {
	Name   string
	OK     bool
	Detail string
	Fix    string
	// Advisory checks are surfaced but never fatal - a failing advisory check
	// does not block `serve` or fail `doctor`.
	Advisory bool
}

type Report struct {
	checks []Check
}

// Checks returns every check in run order, for structured renderers
// (e.g. `kernl doctor --json`).
func (r *Report) Checks() []Check {
	return r.checks
}

func (r *Report) Check(name string) *Check {
	for i := range r.checks {
		if r.checks[i].Name == name {
			return &r.checks[i]
		}
	}
	return nil
}

func (r *Report) AllOK() bool {
	for _, c := range r.checks {
		if !c.OK {
			return false
		}
	}
	return true
}

// RequiredFailed reports whether any non-advisory check failed. This is the
// gate that blocks `serve` and fails `doctor`; advisory failures are shown but
// tolerated.
func (r *Report) RequiredFailed() bool {
	for _, c := range r.checks {
		if !c.OK && !c.Advisory {
			return true
		}
	}
	return false
}

type Deps struct {
	LookPath   func(string) (string, error)
	ConfigPath string
	GoVersion  string
	// Orchestrator reports whether orchestration is enabled. When false (e.g.
	// `serve --no-orchestrator`), the bd CLI is not required and its check is
	// downgraded to advisory.
	Orchestrator bool
	// VaultOrphans lists notes sitting in a kernl-generated folder that no
	// entity claims, each already rendered as "<path> (<reason>)". It is a
	// function, and an optional one: the scan needs the graph, which `serve`
	// has not opened yet when it runs preflight, so only `doctor` supplies it
	// and the check is skipped when nil.
	VaultOrphans func() ([]string, error)
	// CompanionsMissing counts entities whose companion note is gone. Optional
	// and graph-backed for the same reason as VaultOrphans, and supplied by the
	// same caller.
	CompanionsMissing func() (int, error)
}

func Run(deps Deps) *Report {
	var checks []Check

	// bd check. bd backs the orchestrator's issue store, so it is required when
	// orchestration is enabled and advisory otherwise (GUI/graph-only serve).
	bdOK := true
	bdDetail := ""
	bdFix := ""
	if _, err := deps.LookPath("bd"); err != nil {
		bdOK = false
		bdDetail = "bd CLI not found in PATH"
		bdFix = "install bd: see https://github.com/gastownhall/beads"
	}
	checks = append(checks, Check{Name: "bd", OK: bdOK, Detail: bdDetail, Fix: bdFix, Advisory: !deps.Orchestrator})

	// opencode check. opencode is just one of several agent CLIs
	// (claude/codex/gemini); the dispatcher fails loud at run time when a
	// configured agent is missing, so this is advisory, never a startup gate.
	ocOK := true
	ocDetail := ""
	ocFix := ""
	if _, err := deps.LookPath("opencode"); err != nil {
		ocOK = false
		ocDetail = "opencode CLI not found in PATH"
		ocFix = "install opencode (or another configured agent CLI): see https://github.com/anthropics/opencode"
	}
	checks = append(checks, Check{Name: "opencode", OK: ocOK, Detail: ocDetail, Fix: ocFix, Advisory: true})

	// Go version check
	goDetail := ""
	goFix := ""
	goOK := checkGoVersion(deps.GoVersion, &goDetail, &goFix)
	checks = append(checks, Check{Name: "go", OK: goOK, Detail: goDetail, Fix: goFix})

	// Config check
	cfgOK := true
	cfgDetail := ""
	cfgFix := ""
	cfgPath := deps.ConfigPath
	if cfgPath == "" {
		cfgOK = false
		cfgDetail = "no config path provided"
		cfgFix = "run kernl serve --config /path/to/kernl.yaml"
	} else if _, err := os.Stat(cfgPath); err != nil {
		cfgOK = false
		cfgDetail = fmt.Sprintf("config file not found: %s", cfgPath)
		// Two different users hit this: one has no config yet, one has one
		// somewhere else and is running from the wrong directory. Naming only
		// the first sends the second off to create a duplicate.
		cfgFix = fmt.Sprintf("copy kernl.yaml.example to %s and fill in your agents, or export KERNL_CONFIG=<path> if you already have a config elsewhere", cfgPath)
	} else if _, err := config.Load(cfgPath); err != nil {
		cfgOK = false
		cfgDetail = fmt.Sprintf("config invalid: %v", err)
		cfgFix = fmt.Sprintf("fix the errors in %s (hint: kernl doctor shows the issue)", cfgPath)
	}
	checks = append(checks, Check{Name: "config", OK: cfgOK, Detail: cfgDetail, Fix: cfgFix})

	if deps.VaultOrphans != nil {
		checks = append(checks, vaultLayoutCheck(deps.VaultOrphans))
	}

	if deps.CompanionsMissing != nil {
		checks = append(checks, companionNotesCheck(deps.CompanionsMissing))
	}

	return &Report{checks: checks}
}

// companionNotesCheck reports entities whose companion note is gone, which is the
// mirror image of vaultLayoutCheck: that one finds files no entity claims, this
// one finds entities with no file.
//
// Advisory, and for a sharper reason than vault-layout's. The count cannot
// distinguish an entity that never had a companion from one whose note the user
// deleted on purpose, and the second is a choice kernl honours elsewhere. So this
// names the drift and points at the command that repairs it; it never repairs on
// its own, and it never fails the run.
func companionNotesCheck(count func() (int, error)) Check {
	missing, err := count()
	if err != nil {
		return Check{
			Name:     "companion-notes",
			Detail:   fmt.Sprintf("could not scan the graph: %v", err),
			Fix:      "check vault.root in the config and that the graph db is readable",
			Advisory: true,
		}
	}
	if missing == 0 {
		return Check{Name: "companion-notes", OK: true, Advisory: true}
	}
	return Check{
		Name:     "companion-notes",
		Detail:   fmt.Sprintf("%d task/project/bookmark(s) have no companion note, so they exist only in the graph db and not in the vault markdown", missing),
		Fix:      "kernl note backfill-companions --dry-run to see them, then --yes to write them",
		Advisory: true,
	}
}

// vaultLayoutCheck reports notes that live in a folder kernl generates but that
// no entity claims. It is advisory on purpose: a note written by hand inside
// tasks/ is legitimate - the vault belongs to the user - so the drift is worth
// naming and never worth blocking on.
func vaultLayoutCheck(scan func() ([]string, error)) Check {
	orphans, err := scan()
	if err != nil {
		return Check{
			Name:     "vault-layout",
			Detail:   fmt.Sprintf("could not scan the vault: %v", err),
			Fix:      "check vault.root in the config and that the graph db is readable",
			Advisory: true,
		}
	}
	if len(orphans) == 0 {
		return Check{Name: "vault-layout", OK: true, Advisory: true}
	}
	return Check{
		Name:     "vault-layout",
		Detail:   fmt.Sprintf("%d note(s) in kernl-generated folders belong to no entity: %s", len(orphans), strings.Join(orphans, ", ")),
		Fix:      "move them elsewhere in the vault, or leave them - kernl never rewrites a note it did not create",
		Advisory: true,
	}
}

func checkGoVersion(version string, detail, fix *string) bool {
	if version == "" {
		return true // not enforced when not provided
	}
	if len(version) < 4 || version[0] != 'g' || version[1] != 'o' || version[2] < '0' || version[2] > '9' {
		*detail = "unable to parse Go version: " + version
		*fix = "ensure Go 1.24+ is installed: see https://go.dev/dl"
		return false
	}
	major := 0
	minor := 0
	i := 2
	for i < len(version) && version[i] >= '0' && version[i] <= '9' {
		major = major*10 + int(version[i]-'0')
		i++
	}
	if i < len(version) && version[i] == '.' {
		i++
		for i < len(version) && version[i] >= '0' && version[i] <= '9' {
			minor = minor*10 + int(version[i]-'0')
			i++
		}
	}
	if major < 1 || (major == 1 && minor < 24) {
		*detail = "Go version too old: " + version + " (need >= 1.24)"
		*fix = "install Go 1.24+: see https://go.dev/dl"
		return false
	}
	return true
}

// LookPath wraps exec.LookPath for production use.
func LookPath(bin string) (string, error) {
	return exec.LookPath(bin)
}
