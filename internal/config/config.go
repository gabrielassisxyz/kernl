package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type ActionsConfig struct {
	Take            string `yaml:"take,omitempty"`
	Scene           string `yaml:"scene,omitempty"`
	ScopeRefinement string `yaml:"scopeRefinement,omitempty"`
	StaleGrooming   string `yaml:"staleGrooming,omitempty"`
}

type Settings struct {
	Agents   map[string]AgentConfig `yaml:"agents"`
	Actions  ActionsConfig          `yaml:"actions,omitempty"`
	Pools    map[string]PoolConfig  `yaml:"pools"`
	Defaults DefaultsConfig         `yaml:"defaults"`
}

type AgentConfig struct {
	Command      string            `yaml:"command"`
	Args         []string          `yaml:"args,omitempty"`
	Env          map[string]string `yaml:"env,omitempty"`
	Type         string            `yaml:"type,omitempty"`
	Vendor       string            `yaml:"vendor,omitempty"`
	Provider     string            `yaml:"provider,omitempty"`
	AgentName    string            `yaml:"agent_name,omitempty"`
	LeaseModel   string            `yaml:"lease_model,omitempty"`
	Model        string            `yaml:"model,omitempty"`
	Flavor       string            `yaml:"flavor,omitempty"`
	Version      string            `yaml:"version,omitempty"`
	ApprovalMode string            `yaml:"approvalMode,omitempty"`
	Label        string            `yaml:"label,omitempty"`
}

type PoolConfig struct {
	Agents []WeightedAgent `yaml:"agents"`
}

type WeightedAgent struct {
	AgentID string `yaml:"agentId"`
	Weight  int    `yaml:"weight"`
}

type DefaultsConfig struct {
	InteractiveSessionTimeoutMinutes int `yaml:"interactiveSessionTimeoutMinutes"`
}

type RegistryConfig struct {
	Repos []RepoEntry `yaml:"repos"`
}

type RepoEntry struct {
	Path          string `yaml:"path"`
	MemoryManager string `yaml:"memoryManager"`
	// DefaultBranch overrides what epic and bead branches are cut from. Leave
	// it unset and the branch is read out of the repository itself; set it
	// only when the repository disagrees with what origin advertises.
	DefaultBranch string `yaml:"defaultBranch,omitempty"`
	// VerifyCommand is what an agent runs before declaring a stage done. Unset,
	// it is the repository's own bin/ci, which must exist: a repository that
	// cannot say how it is verified gets no work dispatched into it.
	VerifyCommand string `yaml:"verifyCommand,omitempty"`
	// PRTextCommand is how this repository says "this prose is acceptable". It
	// reads the pull request's title and body on stdin and refuses with a
	// non-zero exit, exactly the shape a repository's own prose gate already
	// has. Shipment runs it before opening the pull request, and kernl runs it
	// again over what was published.
	//
	// Unset means this repository declares no rule about prose, which passes.
	// It never means refuse: verifyCommand has a default worth falling back to
	// (bin/ci), and a text gate has none - inventing one would impose kernl's
	// own writing rules on every repository it ships for.
	PRTextCommand string `yaml:"prTextCommand,omitempty"`
	// IrreversibleSurfaces lists the paths this repository considers expensive
	// to undo - migrations, a published API surface, generated artifacts,
	// lockfiles. A branch that touched one sends an integration review
	// rejection to the operator instead of another fix-up round, because only
	// the repository knows what it cannot cheaply take back.
	//
	// A pattern ending in "/" matches that whole subtree; anything else is
	// matched with path.Match against the repository-relative path. Unset means
	// this repository declares nothing, which reads as "no mechanical reason to
	// escalate" - never as "everything here is irreversible".
	IrreversibleSurfaces []string `yaml:"irreversibleSurfaces,omitempty"`
	// ContextDocs lists repository-relative paths, in the order they are
	// read, that the Oracle sees when it writes a decision record's impact
	// field: what this repository is for, since the Oracle itself has no
	// repository access to find that out (see app.AssembleContext). The
	// listed files are sent VERBATIM to whatever llm.agent/llm.endpoint
	// points at - nothing leaves this machine that was not named here.
	// Unset defaults to README.md, then ROADMAP.md; a repository that keeps
	// no ROADMAP.md (this one, on purpose - see AGENTS.md §6) is not a
	// failure, it renders as "not found" in the assembled text.
	ContextDocs []string       `yaml:"contextDocs,omitempty"`
	Shipment    ShipmentConfig `yaml:"shipment,omitempty"`
}

// ShipmentConfig declares where a repository is allowed to publish. It has no
// safe default, so the zero value denies: the empty allow-list refuses to push
// at all. Shipment is the only stage that acts outside the machine, and it must
// be told it may.
type ShipmentConfig struct {
	// AllowedRemotes lists the git remotes shipment may push to, in any of the
	// usual spellings (scp-like, https, ssh://, or host/owner/repo). The push
	// destination and the reported pull request are both checked against it.
	AllowedRemotes []string `yaml:"allowedRemotes,omitempty"`
	// Remote names the git remote to push to. It is resolved to a URL before
	// dispatch and handed to the agent verbatim, so nothing about the
	// destination is left for the agent to work out on its own.
	Remote string `yaml:"remote,omitempty"`
}

type ServerConfig struct {
	Port int `yaml:"port"`
	// Host is the interface to bind. It defaults to loopback: the API has no
	// authentication, so binding every interface would offer the vault and the
	// orchestrator to anything that can reach the machine. Set it to 0.0.0.0
	// deliberately (a container, a trusted LAN), never by accident.
	Host string `yaml:"host,omitempty"`
}

// DefaultFixupBudget is orchestrator.fixupBudget when the operator states
// none.
//
// Three, not one: the cap it replaced allowed a single fix-up round per epic
// and sent the second rejection to a human, on the premise that a reviewer
// rejecting twice is describing something another round will not fix. That
// premise did not hold - two rejections were two different defects, each pass
// fixed what it was named, and the escalation cost a day. Three leaves room
// for a loop that is genuinely converging while still surfacing one that is
// not, on the same day rather than at the end of a budget nobody watches.
const DefaultFixupBudget = 3

type OrchestratorConfig struct {
	WorktreeRoot       string `yaml:"worktreeRoot,omitempty"`
	MaxConcurrentBeads int    `yaml:"maxConcurrentBeads,omitempty"`
	RunStatePath       string `yaml:"runStatePath,omitempty"`
	StageRetryAttempts int    `yaml:"stageRetryAttempts,omitempty"`
	// FixupBudget is how many fix-up rounds one epic may spend on integration
	// review rejections before the next one goes to the operator, however
	// cheap it would be to undo. It is the hard stop that keeps "cheap to
	// reverse" from being an unbounded loop, since a reviewer can always find
	// something. Unset defaults to DefaultFixupBudget.
	FixupBudget int `yaml:"fixupBudget,omitempty"`
	// OpencodeConfigPath overrides the permission allowlist handed to a
	// dispatched opencode agent. It is deliberately not defaulted here: unset
	// means kernl writes and uses its own file, while a path that is set and
	// missing is an operator mistake that must fail loud rather than fall back.
	OpencodeConfigPath string `yaml:"opencodeConfigPath,omitempty"`
}

type SweepConfig struct {
	AutoIntervalSeconds int   `yaml:"auto_interval_seconds"`
	PRStaleWarnDays     int   `yaml:"pr_stale_warn_days"`
	FailureThreshold    int   `yaml:"failure_threshold"`
	BackoffMinutes      []int `yaml:"backoff_minutes"`
}

// VaultConfig holds settings for the notes vault watcher.
type VaultConfig struct {
	// Root is the absolute path to the vault directory.
	// When empty, vault watching is disabled.
	Root string `yaml:"root"`
	// CoalesceWindowMs is the fsnotify quiet period before emitting an event (ms).
	// Default: 300.
	CoalesceWindowMs int `yaml:"coalesceWindowMs,omitempty"`
	// MoveWindowMs is the move/delete correlation window (ms).
	// Default: 1000.
	MoveWindowMs int `yaml:"moveWindowMs,omitempty"`
	// RescanIntervalSec, when >0, triggers a periodic cold-start diff as a
	// safety net for missed fsnotify events. Default: 0 (disabled).
	RescanIntervalSec int `yaml:"rescanIntervalSec,omitempty"`
}

// Enabled reports whether vault watching is configured (root is non-empty).
func (v VaultConfig) Enabled() bool { return v.Root != "" }

// LLMConfig holds settings for the LLM chat providers.
type LLMConfig struct {
	Provider string `yaml:"provider"` // "openai" | "anthropic" | "ollama"
	APIKey   string `yaml:"api_key"`
	Model    string `yaml:"model"`
	// Endpoint is always a base URL (scheme + host, e.g. a local proxy such
	// as http://localhost:4000), never a complete request URL - every
	// consumer of this field appends the provider's own request path onto
	// it, resolved in one place by internal/llmendpoint. Optional; empty
	// falls back to the provider's default base URL.
	Endpoint string `yaml:"endpoint"`
	// Agent, when set, names a settings.agents entry that the run report's
	// oracle is asked through - the agent's own CLI, in one-shot answer mode -
	// instead of the provider above. It changes nothing else: the DA chat
	// keeps using Provider/Endpoint/Model either way.
	//
	// It exists because the two consumers of this block want different
	// things from it. The DA chat needs a streaming, tool-calling API. The
	// oracle needs one paragraph, and the best models available here live
	// behind coding-plan CLIs rather than behind an endpoint that would have
	// to be billed separately to serve them.
	Agent string `yaml:"agent,omitempty"`
}

// IsSet reports whether the LLM is configured (provider is non-empty).
func (l LLMConfig) IsSet() bool { return l.Provider != "" }

// InboxConfig holds settings for the inbox DA pre-processing.
type InboxConfig struct {
	// AutoPrep lets the classifier proactively generate a primer for captures it
	// reads as questions. The manual prep trigger works regardless.
	AutoPrep bool `yaml:"auto_prep,omitempty"`
	// DASubdir is the folder under the vault root where DA-authored notes (preps)
	// are materialized as markdown. Default: "kernl/DA" - under the same
	// namespace as every other note kernl writes, so the rest of the vault stays
	// the user's.
	DASubdir string `yaml:"da_subdir,omitempty"`
	// AutoClassify seeds the runtime auto-classify switch the background
	// classifier reads each tick. A pointer, not a plain bool, because the
	// default is ON: a plain bool cannot tell "user set false" from "absent",
	// so `auto_classify: false` could never be persisted. nil ⇒ default true.
	AutoClassify *bool `yaml:"auto_classify,omitempty"`
}

// AutoClassifyEnabled resolves the tri-state AutoClassify to its effective
// boolean: unset means the default-ON.
func (c InboxConfig) AutoClassifyEnabled() bool {
	return c.AutoClassify == nil || *c.AutoClassify
}

// DAConfig declares the DA - the actor an implementer hands a fork over to
// when it meets a choice the bead, the docs and precedent do not determine
// (the fork gate; see local/artifacts/plans/2026-08-01-composer-context-and-fork-gate-plan.md
// §3). It is its own top-level block, named after the actor rather than
// after the gate, because `kernl da` - a separate, planned interactive
// surface - spawns this same agent in this same place and has to read these
// same two keys. A block named after the gate would guarantee a second
// declaration of the same configuration the day that surface exists.
type DAConfig struct {
	// Agent names a settings.agents key, exactly as llm.agent already does:
	// the command and the model come from that entry, and there is
	// deliberately no model key here - one is not invented for an actor that
	// already has a place to name it.
	Agent string `yaml:"agent,omitempty"`
	// WorkDir is the operator's own system repository - where their telos,
	// notes, recorded preferences and memories already live - and it is the
	// deliberate opposite of the Oracle's rule (see LLMConfig.Agent and
	// app.CLIImpactComposer): the Oracle runs with no working directory at
	// all, because it must never read a repository it is only supposed to
	// describe, while the DA is worthless unless it can reach what the
	// operator has actually written down. Neither is a precedent for the
	// other. It is configuration, and never a literal in the tree: this
	// repository is public, and an absolute path from one machine cannot
	// appear in it.
	WorkDir string `yaml:"workDir,omitempty"`
}

type Config struct {
	Settings     Settings           `yaml:"settings"`
	Registry     RegistryConfig     `yaml:"registry"`
	Server       ServerConfig       `yaml:"server"`
	Orchestrator OrchestratorConfig `yaml:"orchestrator"`
	Sweep        SweepConfig        `yaml:"sweep"`
	Vault        VaultConfig        `yaml:"vault,omitempty"`
	LLM          LLMConfig          `yaml:"llm,omitempty"`
	Inbox        InboxConfig        `yaml:"inbox,omitempty"`
	DA           DAConfig           `yaml:"da,omitempty"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: reading config %s: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: parsing config %s: %w", path, err)
	}

	for i := range cfg.Registry.Repos {
		expanded, err := expandHomePath(cfg.Registry.Repos[i].Path)
		if err != nil {
			return nil, err
		}
		cfg.Registry.Repos[i].Path = expanded
	}

	if err := expandHomePaths(
		&cfg.Orchestrator.WorktreeRoot,
		&cfg.Orchestrator.RunStatePath,
		&cfg.Orchestrator.OpencodeConfigPath,
		&cfg.Vault.Root,
		&cfg.DA.WorkDir,
	); err != nil {
		return nil, err
	}

	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}

	if cfg.Server.Host == "" {
		cfg.Server.Host = "127.0.0.1"
	}

	if cfg.Inbox.DASubdir == "" {
		cfg.Inbox.DASubdir = "kernl/DA"
	}

	if cfg.Settings.Defaults.InteractiveSessionTimeoutMinutes == 0 {
		cfg.Settings.Defaults.InteractiveSessionTimeoutMinutes = 10
	}

	if cfg.Orchestrator.MaxConcurrentBeads == 0 {
		cfg.Orchestrator.MaxConcurrentBeads = 5
	}
	if cfg.Orchestrator.StageRetryAttempts == 0 {
		cfg.Orchestrator.StageRetryAttempts = 2
	}
	if cfg.Orchestrator.FixupBudget == 0 {
		cfg.Orchestrator.FixupBudget = DefaultFixupBudget
	}
	if cfg.Orchestrator.WorktreeRoot == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("KERNL DISPATCH FAILURE: cannot resolve home dir for worktree root default: %w", err)
		}
		cfg.Orchestrator.WorktreeRoot = filepath.Join(home, ".kernl", "worktrees")
	}
	if cfg.Orchestrator.RunStatePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("KERNL DISPATCH FAILURE: cannot resolve home dir for run-state path default: %w", err)
		}
		cfg.Orchestrator.RunStatePath = filepath.Join(home, ".kernl", "runstate.db")
	}

	if cfg.Sweep.AutoIntervalSeconds == 0 {
		cfg.Sweep.AutoIntervalSeconds = 60
	}
	if cfg.Sweep.PRStaleWarnDays == 0 {
		cfg.Sweep.PRStaleWarnDays = 7
	}
	if cfg.Sweep.FailureThreshold == 0 {
		cfg.Sweep.FailureThreshold = 3
	}
	if len(cfg.Sweep.BackoffMinutes) == 0 {
		cfg.Sweep.BackoffMinutes = []int{5, 15, 60}
	}

	if len(cfg.Settings.Agents) == 0 {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: %s defines zero agents under settings.agents - the orchestrator cannot dispatch. Fix: copy kernl.yaml.example and fill in at least one agent. Next: kernl doctor", path)
	}

	if err := validateDAPairing(cfg.DA); err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: %s: %w", path, err)
	}

	return &cfg, nil
}

// validateDAPairing enforces the one rule Load owns about the da: block:
// both keys empty is a normal, supported state (the fork gate is off, and an
// implementer keeps deciding every fork alone, exactly like today), but
// exactly one set is an operator mistake, not a partial configuration - a
// declared da.agent with nowhere to run, or a declared da.workDir with
// nothing to run it, can never produce a working DA. Whether da.agent names a
// real settings.agents entry, and whether da.workDir exists on disk, are
// cross-references and a filesystem fact respectively - checked at
// resolution time by app.NewDA, the same division of labour newOracle
// already draws for llm.agent (Load never dials out to the filesystem to
// validate a foreign key).
func validateDAPairing(da DAConfig) error {
	agentSet := strings.TrimSpace(da.Agent) != ""
	workDirSet := strings.TrimSpace(da.WorkDir) != ""
	if agentSet == workDirSet {
		return nil
	}
	if agentSet {
		return fmt.Errorf("da.agent is set but da.workDir is not - the fork gate needs both or neither: Fix: set da.workDir to the operator's own system repository, or clear da.agent to leave the fork gate off")
	}
	return fmt.Errorf("da.workDir is set but da.agent is not - the fork gate needs both or neither: Fix: set da.agent to a settings.agents key, or clear da.workDir to leave the fork gate off")
}

// expandHomePath expands a leading ~ or ~/ (or ~\) in a path to the user's home directory.
//
// WHY exclusion of ~user: Username expansion (~otheruser/...) is deliberately not
// supported to avoid user lookup dependencies and security surprises across environments.
// Paths starting with ~ followed by non-separator characters (other than ~ alone)
// fail loud with KERNL DISPATCH FAILURE.
//
// WHY exclusion of symlink/existence checks: Resolving symlinks or non-existent paths
// is deliberately out of scope at config load time. Config loading only normalizes path
// representations (~ expansion); filesystem validity is checked downstream by domain
// components (git, beads, etc.) that consume the path.
func expandHomePath(p string) (string, error) {
	if p == "" {
		return "", nil
	}
	if p == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("KERNL DISPATCH FAILURE: cannot resolve home dir to expand path %q: %w", p, err)
		}
		return home, nil
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("KERNL DISPATCH FAILURE: cannot resolve home dir to expand path %q: %w", p, err)
		}
		return filepath.Join(home, p[2:]), nil
	}
	if strings.HasPrefix(p, "~") {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: user-specific tilde expansion (%q) is not supported; use an explicit path", p)
	}
	return p, nil
}

// expandHomePaths expands tilde paths in place for the given string pointers using expandHomePath.
func expandHomePaths(paths ...*string) error {
	for _, p := range paths {
		expanded, err := expandHomePath(*p)
		if err != nil {
			return err
		}
		*p = expanded
	}
	return nil
}
