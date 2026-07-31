package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/gabrielassisxyz/kernl/internal/backend"
	"github.com/gabrielassisxyz/kernl/internal/config"
	"github.com/gabrielassisxyz/kernl/internal/epic"
	"github.com/gabrielassisxyz/kernl/internal/graph"
	"github.com/gabrielassisxyz/kernl/internal/session"
	"github.com/gabrielassisxyz/kernl/internal/terminal"
)

// App is the top-level application container. Its Close() method must be
// called to release resources (Graph DB, terminal sessions, etc.).
// Callers of NewApp should defer a.Close().
type App struct {
	Backend       backend.BackendPort
	Terminal      *terminal.TerminalManager
	SCM           *session.SessionConnectionManager
	Driver        *SessionDriver
	Config        *config.Config
	EpicEvents    *epic.EpicEventHub
	NudgeRegistry *session.NudgeRegistry
	Graph         *graph.Graph

	// StateDir is where kernl writes files that belong to a run rather than to
	// the repository being worked on - the opencode allowlist a dispatched
	// agent runs under, and the agent state store beneath it. It is resolved
	// once here, at the process boundary, so no function inside the dispatch
	// loop resolves a home directory of its own: the ones that did wrote into
	// the operator's real ~/.kernl from every unit test that reached them.
	StateDir string

	// ConfigPath is the kernl.yaml this process loaded. The settings API needs it
	// to write typed field updates back to the file the user actually edits.
	// Empty when the app was built from an in-memory config (tests, harnesses),
	// which makes config writes unavailable rather than silently misdirected.
	ConfigPath string

	// RepoPath is the repository this App's Backend was constructed for -
	// registry.repos[0] for the single-repo `kernl serve` daemon, but whatever
	// --repo resolved to for an orchestrator verb built via NewAppForRepo. A
	// route that needs "the repository this run is about" (the epic run GUI's
	// bead routes) must read this, not Config.Registry.Repos[0]: that index is
	// a fact about the registry, not about which repo this particular App -
	// and the run it is serving a GUI for - was built against.
	RepoPath string

	// autoClassify is the live switch the background inbox classifier reads each
	// tick. It is session-only (a UI toggle, not persisted) and resets to the
	// config default on restart, so it needs its own guarded state rather than
	// riding on Config - the loop and the HTTP handlers race on it otherwise.
	autoClassifyMu sync.RWMutex
	autoClassify   bool
}

// AutoClassifyEnabled reports whether the background inbox classifier should run
// this tick. The classifier loop calls it every tick; the toggle API flips it.
func (a *App) AutoClassifyEnabled() bool {
	a.autoClassifyMu.RLock()
	defer a.autoClassifyMu.RUnlock()
	return a.autoClassify
}

// SetAutoClassify flips the live auto-classify switch. Seeded from config at boot
// and driven by PUT /api/inbox/auto-classify thereafter.
func (a *App) SetAutoClassify(enabled bool) {
	a.autoClassifyMu.Lock()
	defer a.autoClassifyMu.Unlock()
	a.autoClassify = enabled
}

// NewApp builds the application around the first registered repository. The
// server is single-repo by design and this is its constructor.
func NewApp(cfg *config.Config) (*App, error) {
	if len(cfg.Registry.Repos) == 0 {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: no repos registered in config registry - at least one repo path is required - Fix: add a repo to registry.repos in kernl.yaml")
	}
	return NewAppForRepo(cfg, cfg.Registry.Repos[0].Path)
}

// NewAppForRepo builds the application around a named repository, with the
// backend that repository's tracker requires.
//
// The orchestrator verbs call this, because which repository they act on is a
// per-invocation answer (--repo) and the tracker is a property of that answer.
// It used to be neither: the backend was constructed once, always as bd, always
// from repos[0] - so --repo changed the path handed to each call and never the
// implementation receiving it, and naming a br repository ran bd against it.
func NewAppForRepo(cfg *config.Config, repoPath string) (*App, error) {
	if len(cfg.Registry.Repos) == 0 {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: no repos registered in config registry - at least one repo path is required - Fix: add a repo to registry.repos in kernl.yaml")
	}
	if repoPath == "" {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: no repository named for this app - Fix: pass a registered registry.repos[].path")
	}

	be, err := backend.AutoRouteFromConfig(cfg, repoPath)
	if err != nil {
		return nil, err
	}

	tm := terminal.NewTerminalManager(
		terminal.WithMaxSessions(cfg.Orchestrator.MaxConcurrentBeads),
	)

	provider := &terminalSessionProvider{tm: tm}
	scm := session.NewSessionConnectionManager(provider, nil)
	nudges := session.NewNudgeRegistry()

	stateDir, err := DefaultStateDir()
	if err != nil {
		return nil, err
	}

	spawn := execSpawnFunc
	driver := NewSessionDriver(DriverDeps{
		Backend:       be,
		Spawn:         spawn,
		SCM:           scm,
		NudgeRegistry: nudges,
		LogDir:        filepath.Join(stateDir, "logs"),
	})

	graphPath, err := graphDBFilePath(cfg)
	if err != nil {
		return nil, err
	}
	// Single graph database shared with the vault watcher (serve.go), the
	// capture CLI (capture.go), and the decision-record write path
	// (decision_record.go) - all keyed on this same resolved path, since a
	// second independent derivation of it would risk writing to, or reading
	// from, a different file than the rest of the process.
	g, err := graph.Open(context.Background(), graph.Config{Path: graphPath})
	if err != nil {
		return nil, fmt.Errorf("KERNL DISPATCH FAILURE: opening graph: %w", err)
	}

	return &App{
		Backend:       be,
		StateDir:      stateDir,
		Terminal:      tm,
		SCM:           scm,
		Driver:        driver,
		Config:        cfg,
		EpicEvents:    epic.NewEpicEventHub(),
		NudgeRegistry: nudges,
		Graph:         g,
		RepoPath:      repoPath,
		autoClassify:  cfg.Inbox.AutoClassifyEnabled(),
	}, nil
}

// graphDBFileName is the one spelling of the graph database's filename.
// graphDBFilePath and relatedDecisionsForPrompt (decision_relevance.go) each
// join it onto graphDBDir independently - the latter has to, since it needs
// the path BEFORE deciding whether graphDBFilePath's directory-creating side
// effect is even warranted (see that function's own doc comment). Two
// literal spellings of the same filename would let them silently drift: the
// stat guard would check a path the writer never wrote to, "no related
// decisions" would look identical to a genuine absence, and this project has
// already spent real time on exactly that failure shape once (the sweep
// bead, the pr_url metadata key, the autonomous tag filter). One constant
// makes that drift a compile error instead of a silent read-side mismatch.
const graphDBFileName = ".kernl-graph.db"

// graphDBDir resolves the directory the graph database lives in: the vault
// root when configured, else a fallback under the user's home directory.
// Pure - it never touches the filesystem - so a caller that only needs to
// know WHERE the database would be (relatedDecisionsForPrompt checks
// whether it already exists, before deciding whether opening it is even
// worth the side effect) is not forced to create that directory just to
// find out.
func graphDBDir(cfg *config.Config) (string, error) {
	dir := cfg.Vault.Root
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("KERNL DISPATCH FAILURE: cannot resolve home dir for graph db path default: %w", err)
		}
		dir = filepath.Join(home, ".kernl")
	}
	return dir, nil
}

// graphDBFilePath resolves the single graph database file this process
// reads and writes, both keyed on graphDBFileName under graphDBDir. It is a
// function, not inlined at each call site, because it is called from three
// places (NewAppForRepo here, the decision-record write path in
// decision_record.go, and relatedDecisionsForPrompt in
// decision_relevance.go) that must never derive this path differently - a
// node written to one file and read back from another would look like the
// write silently vanished.
func graphDBFilePath(cfg *config.Config) (string, error) {
	dir, err := graphDBDir(cfg)
	if err != nil {
		return "", err
	}
	// Ensure the directory exists before SQLite tries to open the file there -
	// otherwise the open fails with an opaque "unable to open database file"
	// (e.g. a fresh container/data volume where ~/.kernl does not exist yet).
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("KERNL DISPATCH FAILURE: creating graph db dir %s: %w", dir, err)
	}
	return filepath.Join(dir, graphDBFileName), nil
}

func execSpawnFunc(ctx context.Context, cmd string, args []string, cwd string, env []string) (Process, io.Reader, io.Reader, error) {
	c := exec.CommandContext(ctx, cmd, args...)
	if cwd != "" {
		c.Dir = cwd
	}
	if len(env) > 0 {
		// Layer caller overrides ON TOP of the inherited environment so
		// PATH / HOME / etc. survive - otherwise an agent with just
		// OPENCODE_CONFIG=... and nothing else can't find /usr/bin/git,
		// the bd binary, or the user's home dir.
		c.Env = append(os.Environ(), env...)
	}
	stdout, err := c.StdoutPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := c.StderrPipe()
	if err != nil {
		return nil, nil, nil, fmt.Errorf("stderr pipe: %w", err)
	}
	if err := c.Start(); err != nil {
		return nil, nil, nil, fmt.Errorf("start command: %w", err)
	}
	return &osProcess{cmd: c}, stdout, stderr, nil
}

type osProcess struct {
	cmd *exec.Cmd
}

func (p *osProcess) Wait() error { return p.cmd.Wait() }
func (p *osProcess) Kill() error { return p.cmd.Process.Kill() }

type terminalSessionProvider struct {
	tm *terminal.TerminalManager
}

func (p *terminalSessionProvider) GetSessionEntry(id string) (session.SessionInfo, bool) {
	entry, ok := p.tm.GetSession(id)
	if !ok {
		return session.SessionInfo{}, false
	}
	return session.SessionInfo{
		ID:        entry.Session.ID,
		BeadID:    entry.Session.BeadID,
		BeadTitle: entry.Session.BeadTitle,
		RepoPath:  entry.Session.RepoPath,
		Status:    string(entry.Session.Status),
	}, true
}

func (p *terminalSessionProvider) ListSessionIDs() []session.SessionInfo {
	sessions := p.tm.ListSessions()
	result := make([]session.SessionInfo, 0, len(sessions))
	for _, s := range sessions {
		result = append(result, session.SessionInfo{
			ID:        s.ID,
			BeadID:    s.BeadID,
			BeadTitle: s.BeadTitle,
			RepoPath:  s.RepoPath,
			Status:    string(s.Status),
		})
	}
	return result
}

func (p *terminalSessionProvider) PushEvent(id string, evt session.TerminalEvent) {
	p.tm.PushEvent(id, evt)
}

// Close releases resources owned by App, including the Graph DB.
// Callers of NewApp should defer a.Close().
func (a *App) Close() error {
	if a.Graph != nil {
		return a.Graph.Close()
	}
	return nil
}
