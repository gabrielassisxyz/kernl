package session

type DrainPolicy string

const (
	DrainCloseAfterResult DrainPolicy = "close-after-result"
	DrainNeverOpened      DrainPolicy = "never-opened"
	DrainNone             DrainPolicy = ""
)

type ResultDetection string

const (
	ResultDetectionTypeResult   ResultDetection = "type-result"
	ResultDetectionStatusResult ResultDetection = "status-result"
	// ResultDetectionAgentSettled is pi's terminal marker. It does not emit a
	// "result" event at all: a turn closes with "turn_end" and the process
	// closes with "agent_settled", the last line on the stream.
	ResultDetectionAgentSettled ResultDetection = "agent-settled"
	ResultDetectionNone         ResultDetection = ""
)

type TransportKind string

const (
	TransportStdioStreamJSON TransportKind = "stdin-stream-json"
	TransportJSONRPCStdio    TransportKind = "jsonrpc-stdio"
	TransportCLIArg          TransportKind = "cli-arg"
	TransportHTTPServer      TransportKind = "http-server"
	TransportNone            TransportKind = ""
)

type DialectCapabilities struct {
	Interactive             bool
	PromptTransport         TransportKind
	SupportsFollowUp        bool
	SupportsAskUserAutoResp bool
	StdinDrainPolicy        DrainPolicy
	ResultDetection         ResultDetection
	SupportsInteractive     bool
}

var claudeInteractiveCapabilities = DialectCapabilities{
	Interactive:             true,
	PromptTransport:         TransportStdioStreamJSON,
	SupportsFollowUp:        true,
	SupportsAskUserAutoResp: true,
	StdinDrainPolicy:        DrainCloseAfterResult,
	ResultDetection:         ResultDetectionTypeResult,
	SupportsInteractive:     true,
}

// claudeOneShotCapabilities is what the take-loop actually dispatches: claude
// with the prompt on the CLI arg (`-p <prompt>`), per adapter.BuildClaudePromptModeArgs.
// That invocation never opens an interactive channel, so it cannot receive a
// mid-turn follow-up the way claudeInteractiveCapabilities (stream-json over
// stdin) can - claiming SupportsFollowUp here would promise a nudge over a
// transport this process never listens on.
var claudeOneShotCapabilities = DialectCapabilities{
	Interactive:             false,
	PromptTransport:         TransportCLIArg,
	SupportsFollowUp:        false,
	SupportsAskUserAutoResp: false,
	StdinDrainPolicy:        DrainNeverOpened,
	ResultDetection:         ResultDetectionTypeResult,
	SupportsInteractive:     true,
}

var codexOneShotCapabilities = DialectCapabilities{
	Interactive:             false,
	PromptTransport:         TransportCLIArg,
	SupportsFollowUp:        false,
	SupportsAskUserAutoResp: false,
	StdinDrainPolicy:        DrainNeverOpened,
	ResultDetection:         ResultDetectionTypeResult,
	SupportsInteractive:     true,
}

var codexInteractiveCapabilities = DialectCapabilities{
	Interactive:             true,
	PromptTransport:         TransportJSONRPCStdio,
	SupportsFollowUp:        true,
	SupportsAskUserAutoResp: false,
	StdinDrainPolicy:        DrainCloseAfterResult,
	ResultDetection:         ResultDetectionTypeResult,
	SupportsInteractive:     true,
}

var copilotOneShotCapabilities = DialectCapabilities{
	Interactive:             false,
	PromptTransport:         TransportCLIArg,
	SupportsFollowUp:        false,
	SupportsAskUserAutoResp: true,
	StdinDrainPolicy:        DrainNeverOpened,
	ResultDetection:         ResultDetectionTypeResult,
	SupportsInteractive:     true,
}

var copilotInteractiveCapabilities = DialectCapabilities{
	Interactive:             true,
	PromptTransport:         TransportStdioStreamJSON,
	SupportsFollowUp:        true,
	SupportsAskUserAutoResp: true,
	StdinDrainPolicy:        DrainCloseAfterResult,
	ResultDetection:         ResultDetectionTypeResult,
	SupportsInteractive:     true,
}

var opencodeOneShotCapabilities = DialectCapabilities{
	Interactive:             false,
	PromptTransport:         TransportNone,
	SupportsFollowUp:        false,
	SupportsAskUserAutoResp: false,
	StdinDrainPolicy:        DrainNeverOpened,
	ResultDetection:         ResultDetectionNone,
	SupportsInteractive:     true,
}

var opencodeInteractiveCapabilities = DialectCapabilities{
	Interactive:             true,
	PromptTransport:         TransportHTTPServer,
	SupportsFollowUp:        true,
	SupportsAskUserAutoResp: false,
	StdinDrainPolicy:        DrainCloseAfterResult,
	ResultDetection:         ResultDetectionTypeResult,
	SupportsInteractive:     true,
}

var geminiOneShotCapabilities = DialectCapabilities{
	Interactive:             false,
	PromptTransport:         TransportCLIArg,
	SupportsFollowUp:        false,
	SupportsAskUserAutoResp: false,
	StdinDrainPolicy:        DrainNeverOpened,
	ResultDetection:         ResultDetectionStatusResult,
	SupportsInteractive:     true,
}

var geminiInteractiveCapabilities = DialectCapabilities{
	Interactive:             true,
	PromptTransport:         TransportJSONRPCStdio,
	SupportsFollowUp:        true,
	SupportsAskUserAutoResp: true,
	StdinDrainPolicy:        DrainCloseAfterResult,
	ResultDetection:         ResultDetectionTypeResult,
	SupportsInteractive:     true,
}

// piOneShotCapabilities describes `pi -p --mode json <prompt>`, measured
// against pi 0.81: the stream is NDJSON carrying session/turn/message events
// and closes with "agent_settled". There is no interactive counterpart here -
// pi does have a `--mode rpc`, but this repository has not wired a transport
// to it, and adapter.SupportsInteractive reports pi as non-interactive so the
// take-loop refuses it rather than promising a channel nobody opens.
var piOneShotCapabilities = DialectCapabilities{
	Interactive:             false,
	PromptTransport:         TransportCLIArg,
	SupportsFollowUp:        false,
	SupportsAskUserAutoResp: false,
	StdinDrainPolicy:        DrainNeverOpened,
	ResultDetection:         ResultDetectionAgentSettled,
	SupportsInteractive:     false,
}

// agyOneShotCapabilities describes `agy -p <prompt>`, which prints the
// answer as plain text and nothing else: no event stream, so no turn marker
// and no token usage. The process exiting is the only completion signal, and
// SessionRuntime.WaitDrained already provides it.
var agyOneShotCapabilities = DialectCapabilities{
	Interactive:             false,
	PromptTransport:         TransportCLIArg,
	SupportsFollowUp:        false,
	SupportsAskUserAutoResp: false,
	StdinDrainPolicy:        DrainNeverOpened,
	ResultDetection:         ResultDetectionNone,
	SupportsInteractive:     false,
}

func CapabilitiesForDialect(dialect string, interactive bool) DialectCapabilities {
	switch dialect {
	case "claude":
		if interactive {
			return claudeInteractiveCapabilities
		}
		return claudeOneShotCapabilities
	case "codex":
		if interactive {
			return codexInteractiveCapabilities
		}
		return codexOneShotCapabilities
	case "copilot":
		if interactive {
			return copilotInteractiveCapabilities
		}
		return copilotOneShotCapabilities
	case "opencode":
		if interactive {
			return opencodeInteractiveCapabilities
		}
		return opencodeOneShotCapabilities
	case "gemini":
		if interactive {
			return geminiInteractiveCapabilities
		}
		return geminiOneShotCapabilities
	case "pi":
		// interactive is ignored on purpose: there is no interactive pi
		// profile to return, and answering an interactive request with the
		// one-shot profile is what tells the caller (through
		// SupportsInteractive: false) that the channel does not exist.
		return piOneShotCapabilities
	case "agy":
		return agyOneShotCapabilities
	default:
		return codexOneShotCapabilities
	}
}
