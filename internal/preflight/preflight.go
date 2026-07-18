package preflight

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/gabrielassisxyz/kernl/internal/config"
)

type Check struct {
	Name   string
	OK     bool
	Detail string
	Fix    string
	// Advisory checks are surfaced but never fatal — a failing advisory check
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
		cfgFix = fmt.Sprintf("copy kernl.yaml.example to %s and fill in your agents", cfgPath)
	} else if _, err := config.Load(cfgPath); err != nil {
		cfgOK = false
		cfgDetail = fmt.Sprintf("config invalid: %v", err)
		cfgFix = fmt.Sprintf("fix the errors in %s (hint: kernl doctor shows the issue)", cfgPath)
	}
	checks = append(checks, Check{Name: "config", OK: cfgOK, Detail: cfgDetail, Fix: cfgFix})

	return &Report{checks: checks}
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
