package preflight

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"

	"github.com/gabrielassisxyz/kernl/internal/backend"
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
	// `serve --no-orchestrator`), no tracker CLI is required and those checks
	// are downgraded to advisory.
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

	cfgCheck, cfg := configCheck(deps.ConfigPath)

	// The binaries to check come from the configuration, which is why this
	// runs after the config is loaded. A fixed list used to report "bd" and
	// "opencode" to an operator running claude and codex, and say nothing
	// about either: three verdicts, none of them about a binary a run would
	// actually execute.
	checks = append(checks, binaryChecks(configuredBinaries(cfg, deps.Orchestrator), deps.LookPath)...)

	// Go version check
	goDetail := ""
	goFix := ""
	goOK := checkGoVersion(deps.GoVersion, &goDetail, &goFix)
	checks = append(checks, Check{Name: "go", OK: goOK, Detail: goDetail, Fix: goFix})

	checks = append(checks, cfgCheck)

	if deps.VaultOrphans != nil {
		checks = append(checks, vaultLayoutCheck(deps.VaultOrphans))
	}

	if deps.CompanionsMissing != nil {
		checks = append(checks, companionNotesCheck(deps.CompanionsMissing))
	}

	return &Report{checks: checks}
}

// configCheck validates the config file and hands back what it parsed, so the
// checks that depend on the configuration do not read it a second time. A nil
// config means it could not be read: nothing downstream can be checked, and
// the config check itself already says why.
func configCheck(path string) (Check, *config.Config) {
	if path == "" {
		return Check{Name: "config", Detail: "no config path provided", Fix: "run kernl serve --config /path/to/kernl.yaml"}, nil
	}
	if _, err := os.Stat(path); err != nil {
		return Check{
			Name:   "config",
			Detail: fmt.Sprintf("config file not found: %s", path),
			Fix:    fmt.Sprintf("copy kernl.yaml.example to %s and fill in your agents", path),
		}, nil
	}
	cfg, err := config.Load(path)
	if err != nil {
		return Check{
			Name:   "config",
			Detail: fmt.Sprintf("config invalid: %v", err),
			Fix:    fmt.Sprintf("fix the errors in %s (hint: kernl doctor shows the issue)", path),
		}, nil
	}
	return Check{Name: "config", OK: true}, cfg
}

// requiredBinary is one CLI the configuration would execute, and who asked
// for it.
type requiredBinary struct {
	name     string
	usedBy   string
	fix      string
	advisory bool
}

// configuredBinaries lists the CLIs this configuration names: the tracker
// behind each registered repository, and the command of every configured
// agent. Trackers gate a run the way bd always did, so they are required when
// orchestration is on. Agents stay advisory: dispatch fails loud at run time
// when the one it picked is missing, and refusing to serve the graph because
// one of several configured agents is uninstalled would be out of proportion.
func configuredBinaries(cfg *config.Config, orchestrating bool) []requiredBinary {
	if cfg == nil {
		return nil
	}
	var bins []requiredBinary
	seen := map[string]int{}
	add := func(b requiredBinary) {
		if b.name == "" {
			return
		}
		if i, ok := seen[b.name]; ok {
			bins[i].usedBy += ", " + b.usedBy
			bins[i].advisory = bins[i].advisory && b.advisory
			return
		}
		seen[b.name] = len(bins)
		bins = append(bins, b)
	}

	for _, repo := range cfg.Registry.Repos {
		add(requiredBinary{
			name:     backend.TrackerBinary(repo.MemoryManager),
			usedBy:   "repo " + repo.Path,
			fix:      "install the tracker CLI, or correct registry.repos[].memoryManager in kernl.yaml",
			advisory: !orchestrating,
		})
	}

	// Sorted, because map iteration order would otherwise shuffle the doctor
	// output between runs and make two reports impossible to diff.
	agentIDs := make([]string, 0, len(cfg.Settings.Agents))
	for id := range cfg.Settings.Agents {
		agentIDs = append(agentIDs, id)
	}
	sort.Strings(agentIDs)
	for _, id := range agentIDs {
		add(requiredBinary{
			name:     cfg.Settings.Agents[id].Command,
			usedBy:   "agent " + id,
			fix:      "install it, or correct settings.agents." + id + ".command in kernl.yaml",
			advisory: true,
		})
	}
	return bins
}

func binaryChecks(bins []requiredBinary, lookPath func(string) (string, error)) []Check {
	checks := make([]Check, 0, len(bins))
	for _, b := range bins {
		check := Check{Name: b.name, OK: true, Advisory: b.advisory}
		if _, err := lookPath(b.name); err != nil {
			check.OK = false
			check.Detail = fmt.Sprintf("%s not found in PATH (needed by %s)", b.name, b.usedBy)
			check.Fix = b.fix
		}
		checks = append(checks, check)
	}
	return checks
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
