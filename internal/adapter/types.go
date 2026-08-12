package adapter

type AgentTarget struct {
	Command      string
	Model        string
	ApprovalMode string

	// ApprovalBridgePath is the kernl binary an agent spawns to ask for
	// permission. It is an absolute path rather than the bare name because the
	// process resolving it is the agent, not a login shell, and the agent's
	// PATH is whatever kernl was started with.
	ApprovalBridgePath string
	// ApprovalExtensionPath is the file pi loads with -e to gate its own tool
	// calls. claude needs no equivalent: it takes the bridge as an MCP server
	// on the command line, while pi gates from inside an extension hook.
	ApprovalExtensionPath string
}
