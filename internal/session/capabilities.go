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
	default:
		return codexOneShotCapabilities
	}
}
