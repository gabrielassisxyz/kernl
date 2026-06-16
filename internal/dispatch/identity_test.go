package dispatch

import (
	"strings"
	"testing"
)

func TestDetectAgentProviderId(t *testing.T) {
	tests := []struct {
		command string
		want    AgentProviderId
	}{
		{"claude", ProviderClaude},
		{"claude-code", ProviderClaude},
		{"codex", ProviderCodex},
		{"codex-cli", ProviderCodex},
		{"chatgpt", ProviderCodex},
		{"openai", ProviderCodex},
		{"copilot", ProviderCopilot},
		{"github-copilot", ProviderCopilot},
		{"gemini", ProviderGemini},
		{"opencode", ProviderOpenCode},
		{"my-opencode-bin", ProviderOpenCode},
		{"", ProviderUnknown},
		{"unknown-agent", ProviderUnknown},
	}

	for _, tt := range tests {
		got := DetectAgentProviderId(tt.command)
		if got != tt.want {
			t.Errorf("DetectAgentProviderId(%q) = %q, want %q", tt.command, got, tt.want)
		}
	}
}

func TestToCanonicalLeaseIdentity_LabelNotUsedAsAgentName(t *testing.T) {
	canonical := ToCanonicalLeaseIdentity(AgentIdentityLike{
		Command: "codex",
		Label:   "GPT Codex Spark 5.3",
		Model:   "gpt-5.3-codex-spark",
		Version: "5.3",
	})
	if canonical.AgentName != "Codex" {
		t.Errorf("agent_name = %q, want %q", canonical.AgentName, "Codex")
	}
	if canonical.Provider != "Codex" {
		t.Errorf("provider = %q, want %q", canonical.Provider, "Codex")
	}
	if canonical.LeaseModel != "GPT Codex Spark" {
		t.Errorf("lease_model = %q, want %q", canonical.LeaseModel, "GPT Codex Spark")
	}
	if canonical.Version != "5.3" {
		t.Errorf("version = %q, want %q", canonical.Version, "5.3")
	}
	if canonical.AgentType != "cli" {
		t.Errorf("agent_type = %q, want %q", canonical.AgentType, "cli")
	}
}

func TestToCanonicalLeaseIdentity_ExplicitAgentName(t *testing.T) {
	canonical := ToCanonicalLeaseIdentity(AgentIdentityLike{
		Command:   "codex",
		AgentName: "Codex CLI",
		Label:     "GPT Codex Spark 5.3",
		Model:     "gpt-5.3-codex-spark",
		Version:   "5.3",
	})
	if canonical.AgentName != "Codex CLI" {
		t.Errorf("agent_name = %q, want %q", canonical.AgentName, "Codex CLI")
	}
}

func TestToExecutionAgentInfo_Claude(t *testing.T) {
	info := ToExecutionAgentInfo(AgentIdentityLike{
		Command: "claude",
		Model:   "claude-opus-4.6",
		Version: "4.6",
	})
	if info.AgentName != "Claude" {
		t.Errorf("agentName = %q, want %q", info.AgentName, "Claude")
	}
	if info.AgentProvider != "Claude" {
		t.Errorf("agentProvider = %q, want %q", info.AgentProvider, "Claude")
	}
	if info.AgentModel != "Opus" {
		t.Errorf("agentModel = %q, want %q", info.AgentModel, "Opus")
	}
	if info.AgentVersion != "4.6" {
		t.Errorf("agentVersion = %q, want %q", info.AgentVersion, "4.6")
	}
	if info.AgentType != "cli" {
		t.Errorf("agentType = %q, want %q", info.AgentType, "cli")
	}
}

func TestToExecutionAgentInfo_Codex(t *testing.T) {
	info := ToExecutionAgentInfo(AgentIdentityLike{
		Command: "codex",
		Model:   "gpt-5.4-codex",
		Version: "5.4",
	})
	if info.AgentName != "Codex" {
		t.Errorf("agentName = %q, want %q", info.AgentName, "Codex")
	}
	if info.AgentProvider != "Codex" {
		t.Errorf("agentProvider = %q, want %q", info.AgentProvider, "Codex")
	}
	if info.AgentModel != "GPT Codex" {
		t.Errorf("agentModel = %q, want %q", info.AgentModel, "GPT Codex")
	}
	if info.AgentVersion != "5.4" {
		t.Errorf("agentVersion = %q, want %q", info.AgentVersion, "5.4")
	}
}

func TestToExecutionAgentInfo_OpenCode(t *testing.T) {
	info := ToExecutionAgentInfo(AgentIdentityLike{
		Command:  "opencode",
		Provider: "OpenCode",
		Model:    "copilot/anthropic/claude-sonnet-4",
		Version:  "4",
	})
	if info.AgentName != "OpenCode" {
		t.Errorf("agentName = %q, want %q", info.AgentName, "OpenCode")
	}
	if info.AgentProvider != "OpenCode" {
		t.Errorf("agentProvider = %q, want %q", info.AgentProvider, "OpenCode")
	}
	if info.AgentModel != "Copilot Anthropic Claude Sonnet" {
		t.Errorf("agentModel = %q, want %q", info.AgentModel, "Copilot Anthropic Claude Sonnet")
	}
	if info.AgentVersion != "4" {
		t.Errorf("agentVersion = %q, want %q", info.AgentVersion, "4")
	}
}

func TestToExecutionAgentInfo_CanonicalFields(t *testing.T) {
	info := ToExecutionAgentInfo(AgentIdentityLike{
		Command: "codex",
		Label:   "GPT Codex Spark 5.3",
		Model:   "gpt-5.3-codex-spark",
		Version: "5.3",
	})
	if info.AgentName != "Codex" {
		t.Errorf("agentName = %q, want %q", info.AgentName, "Codex")
	}
	if info.AgentProvider != "Codex" {
		t.Errorf("agentProvider = %q, want %q", info.AgentProvider, "Codex")
	}
	if info.AgentModel != "GPT Codex Spark" {
		t.Errorf("agentModel = %q, want %q", info.AgentModel, "GPT Codex Spark")
	}
	if info.AgentVersion != "5.3" {
		t.Errorf("agentVersion = %q, want %q", info.AgentVersion, "5.3")
	}
}

func TestFormatAgentDisplayLabel_WithLabelAndModel(t *testing.T) {
	got := FormatAgentDisplayLabel(AgentIdentityLike{
		Command: "codex",
		Label:   "GPT Codex Spark 5.3",
		Model:   "gpt-5.3-codex-spark",
		Version: "5.3",
	})
	want := "Codex GPT Codex Spark 5.3"
	if got != want {
		t.Errorf("FormatAgentDisplayLabel = %q, want %q", got, want)
	}
}

func TestFormatAgentDisplayLabel_NoLabel(t *testing.T) {
	got := FormatAgentDisplayLabel(AgentIdentityLike{
		Command: "codex-cli",
		Model:   "gpt-5.4-codex",
	})
	want := "Codex GPT Codex 5.4"
	if got != want {
		t.Errorf("FormatAgentDisplayLabel = %q, want %q", got, want)
	}
}

func TestFormatAgentDisplayLabel_LabelLastResort(t *testing.T) {
	got := FormatAgentDisplayLabel(AgentIdentityLike{
		Label: "Custom Agent Display",
	})
	want := "Custom Agent Display"
	if got != want {
		t.Errorf("FormatAgentDisplayLabel = %q, want %q", got, want)
	}
}

func TestClaude1MFastSuffixes(t *testing.T) {
	info := ToExecutionAgentInfo(AgentIdentityLike{
		Command: "claude",
		Model:   "claude-opus-4-7-1m",
	})
	if info.AgentVersion != "4.7" {
		t.Errorf("agentVersion = %q, want %q (bug: 1m should not leak into version)", info.AgentVersion, "4.7")
	}
	if info.AgentModel != "Opus (1M context)" {
		t.Errorf("agentModel = %q, want %q", info.AgentModel, "Opus (1M context)")
	}

	info2 := ToExecutionAgentInfo(AgentIdentityLike{
		Command: "claude",
		Model:   "claude-sonnet-4-5-fast",
	})
	if info2.AgentVersion != "4.5" {
		t.Errorf("agentVersion = %q, want %q", info2.AgentVersion, "4.5")
	}
	if info2.AgentModel != "Sonnet (Fast)" {
		t.Errorf("agentModel = %q, want %q", info2.AgentModel, "Sonnet (Fast)")
	}

	info3 := ToExecutionAgentInfo(AgentIdentityLike{
		Command: "claude",
		Model:   "claude-haiku-4-5",
	})
	if info3.AgentVersion != "4.5" {
		t.Errorf("agentVersion = %q, want %q", info3.AgentVersion, "4.5")
	}
	if info3.AgentModel != "Haiku" {
		t.Errorf("agentModel = %q, want %q", info3.AgentModel, "Haiku")
	}
}

func TestCopilotKeepsCopilotProvider(t *testing.T) {
	info := ToExecutionAgentInfo(AgentIdentityLike{
		Command: "copilot",
		Model:   "claude-sonnet-4-5",
	})
	if info.AgentProvider != "Copilot" {
		t.Errorf("agentProvider = %q, want %q", info.AgentProvider, "Copilot")
	}
	label := FormatAgentDisplayLabel(AgentIdentityLike{
		Command: "copilot",
		Model:   "claude-sonnet-4-5",
	})
	if label != "Copilot Claude Sonnet 4.5" {
		t.Errorf("label = %q, want %q", label, "Copilot Claude Sonnet 4.5")
	}

	info2 := ToExecutionAgentInfo(AgentIdentityLike{
		Command: "copilot",
		Model:   "gpt-5.5",
	})
	if info2.AgentProvider != "Copilot" {
		t.Errorf("agentProvider = %q, want %q", info2.AgentProvider, "Copilot")
	}
	label2 := FormatAgentDisplayLabel(AgentIdentityLike{
		Command: "copilot",
		Model:   "gpt-5.5",
	})
	if label2 != "Copilot GPT 5.5" {
		t.Errorf("label = %q, want %q", label2, "Copilot GPT 5.5")
	}

	info3 := ToExecutionAgentInfo(AgentIdentityLike{
		Command: "copilot",
		Model:   "gemini-2.5-pro",
	})
	if info3.AgentProvider != "Copilot" {
		t.Errorf("agentProvider = %q, want %q", info3.AgentProvider, "Copilot")
	}
	label3 := FormatAgentDisplayLabel(AgentIdentityLike{
		Command: "copilot",
		Model:   "gemini-2.5-pro",
	})
	if label3 != "Copilot Gemini Pro 2.5" {
		t.Errorf("label = %q, want %q", label3, "Copilot Gemini Pro 2.5")
	}
}

func checkMatrix(t *testing.T, command, model string, expectNormalize NormalizedAgentIdentity, expectLabel string, expectPills []string) {
	t.Helper()
	n := NormalizeAgentIdentity(AgentIdentityLike{Command: command, Model: model})
	if n.Provider != expectNormalize.Provider {
		t.Errorf("normalize(%q,%q).Provider = %q, want %q", command, model, n.Provider, expectNormalize.Provider)
	}
	if n.Model != expectNormalize.Model {
		t.Errorf("normalize(%q,%q).Model = %q, want %q", command, model, n.Model, expectNormalize.Model)
	}
	if n.Flavor != expectNormalize.Flavor {
		t.Errorf("normalize(%q,%q).Flavor = %q, want %q", command, model, n.Flavor, expectNormalize.Flavor)
	}
	if n.Version != expectNormalize.Version {
		t.Errorf("normalize(%q,%q).Version = %q, want %q", command, model, n.Version, expectNormalize.Version)
	}
	label := FormatAgentDisplayLabel(AgentIdentityLike{Command: command, Model: model})
	if label != expectLabel {
		t.Errorf("label(%q,%q) = %q, want %q", command, model, label, expectLabel)
	}
	if expectPills != nil {
		parts := ParseAgentDisplayParts(AgentIdentityLike{Command: command, Model: model})
		if len(parts.Pills) != len(expectPills) {
			t.Errorf("pills(%q,%q) = %v, want %v", command, model, parts.Pills, expectPills)
		} else {
			for i, p := range parts.Pills {
				if p != expectPills[i] {
					t.Errorf("pills[%d] = %q, want %q", i, p, expectPills[i])
				}
			}
		}
	}
}

func TestCodexExtractorMatrix(t *testing.T) {
	checkMatrix(t, "codex", "gpt-5.5",
		NormalizedAgentIdentity{Provider: "Codex", Model: "GPT", Version: "5.5"},
		"Codex GPT 5.5", []string{"cli"})
	checkMatrix(t, "codex", "gpt-5.4",
		NormalizedAgentIdentity{Provider: "Codex", Model: "GPT", Version: "5.4"},
		"Codex GPT 5.4", nil)
	checkMatrix(t, "codex", "gpt-5.4-mini",
		NormalizedAgentIdentity{Provider: "Codex", Model: "GPT", Flavor: "Mini", Version: "5.4"},
		"Codex GPT Mini 5.4", nil)
	checkMatrix(t, "codex", "gpt-5.3-codex-spark",
		NormalizedAgentIdentity{Provider: "Codex", Model: "GPT", Flavor: "Codex Spark", Version: "5.3"},
		"Codex GPT Codex Spark 5.3", nil)
	checkMatrix(t, "codex", "gpt-5.3-codex-mini",
		NormalizedAgentIdentity{Provider: "Codex", Model: "GPT", Flavor: "Codex Mini", Version: "5.3"},
		"Codex GPT Codex Mini 5.3", nil)
	checkMatrix(t, "codex", "gpt-5-codex-max",
		NormalizedAgentIdentity{Provider: "Codex", Model: "GPT", Flavor: "Codex Max", Version: "5"},
		"Codex GPT Codex Max 5", nil)
	checkMatrix(t, "codex", "chatgpt-5.5",
		NormalizedAgentIdentity{Provider: "Codex", Model: "ChatGPT", Version: "5.5"},
		"Codex ChatGPT 5.5", nil)
}

func TestClaudeExtractorMatrix(t *testing.T) {
	checkMatrix(t, "claude", "claude-opus-4-7",
		NormalizedAgentIdentity{Provider: "Claude", Model: "Claude", Flavor: "Opus", Version: "4.7"},
		"Claude Opus 4.7", nil)
	checkMatrix(t, "claude", "claude-sonnet-4-6",
		NormalizedAgentIdentity{Provider: "Claude", Model: "Claude", Flavor: "Sonnet", Version: "4.6"},
		"Claude Sonnet 4.6", nil)
	checkMatrix(t, "claude", "claude-haiku-4-5",
		NormalizedAgentIdentity{Provider: "Claude", Model: "Claude", Flavor: "Haiku", Version: "4.5"},
		"Claude Haiku 4.5", nil)
	checkMatrix(t, "claude", "claude-opus-4-7-1m",
		NormalizedAgentIdentity{Provider: "Claude", Model: "Claude", Flavor: "Opus (1M context)", Version: "4.7"},
		"Claude Opus (1M context) 4.7", nil)
	checkMatrix(t, "claude", "claude-sonnet-4-5-fast",
		NormalizedAgentIdentity{Provider: "Claude", Model: "Claude", Flavor: "Sonnet (Fast)", Version: "4.5"},
		"Claude Sonnet (Fast) 4.5", nil)
}

func TestGeminiExtractorMatrix(t *testing.T) {
	checkMatrix(t, "gemini", "gemini-2.5-pro",
		NormalizedAgentIdentity{Provider: "Gemini", Model: "Gemini", Flavor: "Pro", Version: "2.5"},
		"Gemini Pro 2.5", nil)
	checkMatrix(t, "gemini", "gemini-2.5-flash",
		NormalizedAgentIdentity{Provider: "Gemini", Model: "Gemini", Flavor: "Flash", Version: "2.5"},
		"Gemini Flash 2.5", nil)
	checkMatrix(t, "gemini", "gemini-2.5-flash-lite",
		NormalizedAgentIdentity{Provider: "Gemini", Model: "Gemini", Flavor: "Flash Lite", Version: "2.5"},
		"Gemini Flash Lite 2.5", nil)
	checkMatrix(t, "gemini", "gemini-3-pro-preview",
		NormalizedAgentIdentity{Provider: "Gemini", Model: "Gemini", Flavor: "Pro (Preview)", Version: "3"},
		"Gemini Pro (Preview) 3", nil)
}

func TestCopilotExtractorMatrix(t *testing.T) {
	checkMatrix(t, "copilot", "claude-sonnet-4-5",
		NormalizedAgentIdentity{Provider: "Copilot", Model: "Claude", Flavor: "Sonnet", Version: "4.5"},
		"Copilot Claude Sonnet 4.5", []string{"copilot", "cli"})
	checkMatrix(t, "copilot", "gpt-5.5",
		NormalizedAgentIdentity{Provider: "Copilot", Model: "GPT", Version: "5.5"},
		"Copilot GPT 5.5", []string{"copilot", "cli"})
	checkMatrix(t, "copilot", "gemini-2.5-pro",
		NormalizedAgentIdentity{Provider: "Copilot", Model: "Gemini", Flavor: "Pro", Version: "2.5"},
		"Copilot Gemini Pro 2.5", []string{"copilot", "cli"})
	checkMatrix(t, "copilot", "gpt-5.3-codex",
		NormalizedAgentIdentity{Provider: "Copilot", Model: "GPT", Flavor: "Codex", Version: "5.3"},
		"Copilot GPT Codex 5.3", []string{"copilot", "cli"})
}

func TestOpenCodeExtractor3Segment(t *testing.T) {
	checkMatrix(t, "opencode", "openrouter/moonshotai/kimi-k2.6",
		NormalizedAgentIdentity{Provider: "OpenCode", Model: "OpenRouter MoonshotAI Kimi-k", Version: "2.6"},
		"OpenCode OpenRouter MoonshotAI Kimi-k 2.6", []string{"openrouter", "cli"})
	checkMatrix(t, "opencode", "openrouter/anthropic/claude-sonnet-4-5",
		NormalizedAgentIdentity{Provider: "OpenCode", Model: "OpenRouter Anthropic Claude Sonnet", Version: "4.5"},
		"OpenCode OpenRouter Anthropic Claude Sonnet 4.5", []string{"openrouter", "cli"})
	checkMatrix(t, "opencode", "openrouter/z-ai/glm-5.1",
		NormalizedAgentIdentity{Provider: "OpenCode", Model: "OpenRouter Z-AI GLM", Version: "5.1"},
		"OpenCode OpenRouter Z-AI GLM 5.1", []string{"openrouter", "cli"})
	checkMatrix(t, "opencode", "openrouter/mistralai/devstral-2512",
		NormalizedAgentIdentity{Provider: "OpenCode", Model: "OpenRouter MistralAI Devstral", Version: "2512"},
		"OpenCode OpenRouter MistralAI Devstral 2512", []string{"openrouter", "cli"})
	checkMatrix(t, "opencode", "openrouter/minimax/minimax-m2.7",
		NormalizedAgentIdentity{Provider: "OpenCode", Model: "OpenRouter Minimax Minimax-m", Version: "2.7"},
		"OpenCode OpenRouter Minimax Minimax-m 2.7", []string{"openrouter", "cli"})
	checkMatrix(t, "opencode", "openrouter/google/gemini-2.5-pro",
		NormalizedAgentIdentity{Provider: "OpenCode", Model: "OpenRouter Google Gemini Pro", Version: "2.5"},
		"OpenCode OpenRouter Google Gemini Pro 2.5", []string{"openrouter", "cli"})
}

func TestOpenCodeExtractorBareAnd2Segment(t *testing.T) {
	checkMatrix(t, "opencode", "kimi-k2.6",
		NormalizedAgentIdentity{Provider: "OpenCode", Model: "Kimi-k", Version: "2.6"},
		"OpenCode Kimi-k 2.6", []string{"opencode", "cli"})
	checkMatrix(t, "opencode", "mistral/devstral-2512",
		NormalizedAgentIdentity{Provider: "OpenCode", Model: "Mistral Devstral", Version: "2512"},
		"OpenCode Mistral Devstral 2512", []string{"cli"})
}

func TestOpenCodeVersionAntiLeak(t *testing.T) {
	info := ToExecutionAgentInfo(AgentIdentityLike{
		Command:  "opencode",
		Provider: "OpenCode",
		Model:    "openrouter/moonshotai/kimi-k2.6",
		Version:  "4.7",
	})
	if info.AgentVersion != "2.6" {
		t.Errorf("version should come from model path (2.6), not leaked binary version; got %q", info.AgentVersion)
	}

	info2 := ToExecutionAgentInfo(AgentIdentityLike{
		Command:  "opencode",
		Provider: "OpenCode",
		Model:    "bare",
		Version:  "4.7",
	})
	if info2.AgentVersion != "4.7" {
		t.Errorf("when model path has no version, should fall back to config version; got %q", info2.AgentVersion)
	}
}

func TestOpenCodePathParsing_Malformed(t *testing.T) {
	parts := ParseOpenCodePath("")
	if parts.Model != "" {
		t.Errorf("empty model should yield empty model, got %q", parts.Model)
	}
	parts = ParseOpenCodePath("///")
	if parts.Model == "" {
		t.Error("malformed slashes should still produce output")
	}
}

func TestFormatLeaseModelDisplay_DropsModelEqualsProvider(t *testing.T) {
	result := FormatLeaseModelDisplay(AgentOptionSeed{
		Provider: "Claude",
		Model:    "Claude",
		Flavor:   "Opus",
	})
	if result != "Opus" {
		t.Errorf("FormatLeaseModelDisplay(Claude/Claude/Opus) = %q, want %q", result, "Opus")
	}

	result2 := FormatLeaseModelDisplay(AgentOptionSeed{
		Provider: "Codex",
		Model:    "GPT",
		Flavor:   "Codex",
	})
	if result2 != "GPT Codex" {
		t.Errorf("FormatLeaseModelDisplay(Codex/GPT/Codex) = %q, want %q", result2, "GPT Codex")
	}
}

func TestFormatAgentFamily(t *testing.T) {
	family := FormatAgentFamily(AgentOptionSeed{
		Provider: "Claude",
		Model:    "Claude",
		Flavor:   "Opus",
	})
	if family != "Claude Opus" {
		t.Errorf("FormatAgentFamily(Claude/Claude/Opus) = %q, want %q", family, "Claude Opus")
	}

	family2 := FormatAgentFamily(AgentOptionSeed{
		Provider: "Codex",
		Model:    "GPT",
		Flavor:   "Codex",
	})
	if family2 != "Codex GPT Codex" {
		t.Errorf("FormatAgentFamily(Codex/GPT/Codex) = %q, want %q", family2, "Codex GPT Codex")
	}
}

func TestSplitOpenCodeModelToken(t *testing.T) {
	tests := []struct {
		input string
		name  string
		ver   string
		tail  string
	}{
		{"claude-sonnet-4-5", "Claude Sonnet", "4.5", ""},
		{"kimi-k2.6", "Kimi-k", "2.6", ""},
		{"glm-5.1", "GLM", "5.1", ""},
		{"devstral-2512", "Devstral", "2512", ""},
		{"gemini-2.5-pro", "Gemini", "2.5", "Pro"},
		{"kimi", "Kimi", "", ""},
	}

	for _, tt := range tests {
		split := SplitOpenCodeModelToken(tt.input)
		if split.Name != tt.name {
			t.Errorf("SplitOpenCodeModelToken(%q).Name = %q, want %q", tt.input, split.Name, tt.name)
		}
		if split.Version != tt.ver {
			t.Errorf("SplitOpenCodeModelToken(%q).Version = %q, want %q", tt.input, split.Version, tt.ver)
		}
		if split.Tail != tt.tail {
			t.Errorf("SplitOpenCodeModelToken(%q).Tail = %q, want %q", tt.input, split.Tail, tt.tail)
		}
	}
}

func TestFormatOpenCodeSegment(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"openrouter", "OpenRouter"},
		{"moonshotai", "MoonshotAI"},
		{"z-ai", "Z-AI"},
		{"anthropic", "Anthropic"},
		{"mistral", "Mistral"},
		{"mistralai", "MistralAI"},
		{"google", "Google"},
		{"opencode", "OpenCode"},
		{"openai", "OpenAI"},
		{"deepseek", "DeepSeek"},
		{"xai", "xAI"},
		{"glm", "GLM"},
		{"llama", "Llama"},
		{"bert", "BERT"},
		{"someothervendorai", "SomeothervendorAI"},
	}

	for _, tt := range tests {
		got := formatOpenCodeSegment(tt.input)
		if got != tt.want {
			t.Errorf("formatOpenCodeSegment(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestProviderLabel(t *testing.T) {
	if v := providerLabel("Claude", ""); v != "Claude" {
		t.Errorf("providerLabel(Claude, empty) = %q, want %q", v, "Claude")
	}
	if v := providerLabel("", "claude"); v != "Claude" {
		t.Errorf("providerLabel(empty, claude) = %q, want %q", v, "Claude")
	}
	if v := providerLabel("", "unknown-bin"); v != "" {
		t.Errorf("providerLabel(empty, unknown-bin) = %q, want empty", v)
	}
}

func TestDisplayCommandLabel(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"claude", "Claude"},
		{"codex-cli", "Codex"},
		{"opencode", "OpenCode"},
		{"gemini", "Gemini"},
		{"copilot", "Copilot"},
		{"", ""},
		{"something-else", ""},
	}

	for _, tt := range tests {
		got := displayCommandLabel(tt.input)
		if got != tt.want {
			t.Errorf("displayCommandLabel(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestCleanValue(t *testing.T) {
	if v := cleanValue("  hello  "); v != "hello" {
		t.Errorf("cleanValue(`  hello  `) = %q, want %q", v, "hello")
	}
	if v := cleanValue(""); v != "" {
		t.Errorf("cleanValue(empty) = %q, want empty", v)
	}
	if v := cleanValue("   "); v != "" {
		t.Errorf("cleanValue(spaces) = %q, want empty", v)
	}
	if v := cleanValue("hello world"); v != "hello world" {
		t.Errorf("cleanValue(`hello world`) = %q, want %q", v, "hello world")
	}
}

func TestGeminiEmptyModel(t *testing.T) {
	n := NormalizeAgentIdentity(AgentIdentityLike{Command: "gemini"})
	if n.Model != "Gemini" {
		t.Errorf("Gemini with no model should default to 'Gemini', got %q", n.Model)
	}
}

func TestLeaseModelDropsRedundantModel(t *testing.T) {
	li := ToCanonicalLeaseIdentity(AgentIdentityLike{
		Command: "claude",
		Model:   "claude-opus-4.6",
	})
	if li.LeaseModel != "Opus" {
		t.Errorf("Claude model='Claude' should be dropped from lease_model; got %q, want %q", li.LeaseModel, "Opus")
	}
}

func TestGeminiPreviewFlavor(t *testing.T) {
	n := NormalizeAgentIdentity(AgentIdentityLike{
		Command: "gemini",
		Model:   "gemini-3-pro-preview",
	})
	if n.Flavor != "Pro (Preview)" {
		t.Errorf("Gemini pro-preview flavor = %q, want %q", n.Flavor, "Pro (Preview)")
	}
}

func TestOpenCodeEmptyModel(t *testing.T) {
	parts := ParseAgentDisplayParts(AgentIdentityLike{Command: "opencode"})
	if parts.Label != "OpenCode" {
		t.Errorf("OpenCode with no model should label 'OpenCode', got %q", parts.Label)
	}
	if len(parts.Pills) != 1 || parts.Pills[0] != "cli" {
		t.Errorf("OpenCode with no model should have pills [cli], got %v", parts.Pills)
	}
}

func TestNormzlizeCodexNoModel(t *testing.T) {
	n := NormalizeAgentIdentity(AgentIdentityLike{Command: "codex"})
	if n.Provider != "Codex" {
		t.Errorf("Codex with no model should still have provider 'Codex', got %q", n.Provider)
	}
}

func TestFormatLeaseModelDisplay_Empty(t *testing.T) {
	result := FormatLeaseModelDisplay(AgentOptionSeed{})
	if result != "" {
		t.Errorf("FormatLeaseModelDisplay(empty) = %q, want empty", result)
	}
}

func TestNormalizeClaudeNoModel(t *testing.T) {
	n := NormalizeAgentIdentity(AgentIdentityLike{Command: "claude"})
	if n.Provider != "Claude" {
		t.Errorf("Claude with no model should have provider 'Claude', got %q", n.Provider)
	}
}

func TestBuildAgentOptionId(t *testing.T) {
	id := BuildAgentOptionID("agent1", AgentOptionSeed{ModelID: "gpt-5-codex"})
	if id != "agent1-gpt-5-codex" {
		t.Errorf("BuildAgentOptionID with modelId = %q, want %q", id, "agent1-gpt-5-codex")
	}

	id2 := BuildAgentOptionID("agent1", AgentOptionSeed{Model: "GPT 5.4", Flavor: "Codex", Version: "5.4"})
	if !isAgentOptionIDValid(id2) {
		t.Errorf("BuildAgentOptionID fallback should produce a clean id, got %q", id2)
	}
}

func isAgentOptionIDValid(id string) bool {
	return len(id) > 0 && id == strings.ToLower(strings.Map(sanitizeRune, id))
}

func sanitizeRune(r rune) rune {
	if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
		return r
	}
	return '-'
}

func TestParseOpenCodeModelSelection_SlplitsProviderFromModelID(t *testing.T) {
	result, err := ParseOpenCodeModelSelection("openrouter/z-ai/glm-5.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ProviderID != "openrouter" {
		t.Errorf("providerID = %q, want %q", result.ProviderID, "openrouter")
	}
	if result.ModelID != "z-ai/glm-5.1" {
		t.Errorf("modelID = %q, want %q", result.ModelID, "z-ai/glm-5.1")
	}
}

func TestParseOpenCodeModelSelection_OmitsEmpty(t *testing.T) {
	result, err := ParseOpenCodeModelSelection("")
	if err != nil {
		t.Fatalf("unexpected error for empty: %v", err)
	}
	if result != nil {
		t.Errorf("empty input should return nil, got %+v", result)
	}

	result, err = ParseOpenCodeModelSelection("  ")
	if err != nil {
		t.Fatalf("unexpected error for whitespace: %v", err)
	}
	if result != nil {
		t.Errorf("whitespace input should return nil, got %+v", result)
	}
}

func TestParseOpenCodeModelSelection_RejectsInvalid(t *testing.T) {
	_, err := ParseOpenCodeModelSelection("glm-5.1")
	if err == nil {
		t.Fatal("expected error for model without provider slash, got nil")
	}
	if !strings.Contains(err.Error(), "expected <providerID>/<modelID>") {
		t.Errorf("error should mention expected format, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "KERNL DISPATCH FAILURE") {
		t.Errorf("error should contain KERNL DISPATCH FAILURE marker, got %q", err.Error())
	}
}

func TestParseOpenCodeModelSelection_TwoSegment(t *testing.T) {
	result, err := ParseOpenCodeModelSelection("anthropic/claude-sonnet-4-5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ProviderID != "anthropic" {
		t.Errorf("providerID = %q, want %q", result.ProviderID, "anthropic")
	}
	if result.ModelID != "claude-sonnet-4-5" {
		t.Errorf("modelID = %q, want %q", result.ModelID, "claude-sonnet-4-5")
	}
}

func TestParseOpenCodeModelSelection_LeadingSlash(t *testing.T) {
	_, err := ParseOpenCodeModelSelection("/model")
	if err == nil {
		t.Fatal("expected error for leading-slash model, got nil")
	}
}

func TestParseOpenCodeModelSelection_TrailingSlash(t *testing.T) {
	_, err := ParseOpenCodeModelSelection("provider/")
	if err == nil {
		t.Fatal("expected error for trailing-slash model, got nil")
	}
}
