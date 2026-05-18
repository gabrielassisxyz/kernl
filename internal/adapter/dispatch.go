package adapter

import (
	"fmt"
)

const TerminalDispatchFailureMarker = "KERNL DISPATCH FAILURE"

type TerminalDispatchKind string

const (
	DispatchKindTake  TerminalDispatchKind = "take"
	DispatchKindScene TerminalDispatchKind = "scene"
)

func DispatchKindFromParent(effectiveParent bool) TerminalDispatchKind {
	if effectiveParent {
		return DispatchKindScene
	}
	return DispatchKindTake
}

func FormatTakeSceneOneShotFailure(dialect AgentDialect, dispatchKind TerminalDispatchKind, transport string) string {
	return fmt.Sprintf(
		"%s: %s dispatch for %s resolved to %s transport. One-shot cli-arg execution is forbidden for take and scene sessions. Configure an interactive agent transport for this provider.",
		TerminalDispatchFailureMarker,
		dispatchKind,
		dialect,
		transport,
	)
}

func AssertTakeSceneInteractiveCapabilities(dialect AgentDialect, dispatchKind TerminalDispatchKind, interactive bool, transport string) error {
	if interactive && transport != "cli-arg" {
		return nil
	}
	return fmt.Errorf("%s", FormatTakeSceneOneShotFailure(dialect, dispatchKind, transport))
}

func ResolveInteractiveTransport(dialect AgentDialect, isInteractive bool) string {
	if !isInteractive {
		return "cli-arg"
	}
	switch dialect {
	case DialectCodex:
		return "jsonrpc-stdio"
	case DialectCopilot:
		return "stdin-stream-json"
	case DialectOpenCode:
		return "http-server"
	case DialectGemini:
		return "acp-stdio"
	default:
		return "stdin-stream-json"
	}
}

func ResolveTakeSceneRuntime(dialect AgentDialect, dispatchKind TerminalDispatchKind) (interactive bool, transport string, err error) {
	isInteractive := SupportsInteractive(dialect)
	transport = ResolveInteractiveTransport(dialect, isInteractive)
	if err := AssertTakeSceneInteractiveCapabilities(dialect, dispatchKind, isInteractive, transport); err != nil {
		return false, "", err
	}
	return isInteractive, transport, nil
}

func SupportsInteractive(dialect AgentDialect) bool {
	switch dialect {
	case DialectClaude, DialectCodex, DialectCopilot, DialectOpenCode, DialectGemini:
		return true
	default:
		return false
	}
}