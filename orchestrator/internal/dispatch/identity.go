package dispatch

import (
	"fmt"
	"regexp"
	"strings"
)

type AgentProviderId string

const (
	ProviderClaude  AgentProviderId = "Claude"
	ProviderCopilot AgentProviderId = "Copilot"
	ProviderCodex   AgentProviderId = "Codex"
	ProviderGemini  AgentProviderId = "Gemini"
	ProviderOpenCode AgentProviderId = "OpenCode"
	ProviderUnknown AgentProviderId = "unknown"
)

func DetectAgentProviderId(command string) AgentProviderId {
	lower := strings.TrimSpace(strings.ToLower(command))
	if lower == "" {
		return ProviderUnknown
	}
	if strings.Contains(lower, "opencode") {
		return ProviderOpenCode
	}
	if strings.Contains(lower, "copilot") {
		return ProviderCopilot
	}
	if strings.Contains(lower, "claude") {
		return ProviderClaude
	}
	if strings.Contains(lower, "codex") || strings.Contains(lower, "chatgpt") || strings.Contains(lower, "openai") {
		return ProviderCodex
	}
	if strings.Contains(lower, "gemini") {
		return ProviderGemini
	}
	return ProviderUnknown
}

func providerLabel(provider, command string) string {
	cleaned := cleanValue(provider)
	if cleaned != "" {
		return cleaned
	}
	detected := DetectAgentProviderId(command)
	if detected == ProviderUnknown {
		return ""
	}
	return string(detected)
}

type AgentIdentityLike struct {
	Command   string
	Provider  string
	Model     string
	Flavor    string
	Version   string
	Label     string
	AgentType string
	Vendor    string
	AgentName string
	LeaseModel string
	Kind      string
}

func NewAgentIdentityFromCommand(command string) AgentIdentityLike {
	return AgentIdentityLike{Command: command}
}

type NormalizedAgentIdentity struct {
	Provider string
	Model    string
	Flavor   string
	Version  string
}

type CanonicalLeaseIdentity struct {
	AgentType  string `json:"agent_type,omitempty"`
	Vendor     string `json:"vendor,omitempty"`
	Provider   string `json:"provider,omitempty"`
	AgentName  string `json:"agent_name,omitempty"`
	LeaseModel string `json:"lease_model,omitempty"`
	Version    string ` json:"version,omitempty"`
}

type ExecutionAgentInfo struct {
	AgentName     string `json:"agentName"`
	AgentProvider string `json:"agentProvider"`
	AgentModel    string `json:"agentModel"`
	AgentVersion  string `json:"agentVersion"`
	AgentType     string `json:"agentType"`
}

type AgentOptionSeed struct {
	Provider string
	Model    string
	Flavor   string
	Version  string
	ModelID  string
}

type AgentDisplayParts struct {
	Label string
	Pills []string
}

func ToCanonicalLeaseIdentity(agent AgentIdentityLike) CanonicalLeaseIdentity {
	core := buildCanonicalCore(agent)
	result := CanonicalLeaseIdentity{}
	if core.AgentType != "" {
		result.AgentType = core.AgentType
	}
	if core.Vendor != "" && core.Vendor != "unknown" {
		result.Vendor = core.Vendor
	}
	if core.Provider != "" {
		result.Provider = core.Provider
	}
	if core.AgentName != "" {
		result.AgentName = core.AgentName
	}
	if core.LeaseModel != "" {
		result.LeaseModel = core.LeaseModel
	}
	if core.Version != "" {
		result.Version = core.Version
	}
	return result
}

func ToExecutionAgentInfo(agent AgentIdentityLike) ExecutionAgentInfo {
	canonical := ToCanonicalLeaseIdentity(agent)
	return ExecutionAgentInfo{
		AgentName:     canonical.AgentName,
		AgentProvider: canonical.Provider,
		AgentModel:    canonical.LeaseModel,
		AgentVersion:  canonical.Version,
		AgentType:     canonical.AgentType,
	}
}

type canonicalCore struct {
	Normalized NormalizedAgentIdentity
	AgentType string
	Vendor    string
	Provider  string
	AgentName string
	LeaseModel string
	Version   string
}

func buildCanonicalCore(agent AgentIdentityLike) canonicalCore {
	n := NormalizeAgentIdentity(agent)
	explicitCommand := cleanValue(agent.Command)
	agentType := cleanValue(agent.AgentType)
	if agentType == "" {
		agentType = cleanValue(agent.Kind)
	}
	if agentType == "" {
		agentType = "cli"
	}
	vendor := cleanValue(agent.Vendor)
	if vendor == "" && explicitCommand != "" {
		vendor = string(DetectAgentProviderId(explicitCommand))
	}
	provider := n.Provider
	if provider == "" {
		provider = cleanValue(agent.Provider)
	}
	agentName := cleanValue(agent.AgentName)
	if agentName == "" {
		agentName = displayCommandLabel(explicitCommand)
	}
	if agentName == "" {
		agentName = provider
	}
	if agentName == "" {
		agentName = explicitCommand
	}
	if agentName == "" {
		agentName = "Unknown"
	}
	derivedLeaseModel := FormatLeaseModelDisplay(AgentOptionSeed{
		Provider: n.Provider,
		Model:    n.Model,
		Flavor:   n.Flavor,
	})
	if derivedLeaseModel == "" {
		derivedLeaseModel = cleanValue(agent.Model)
	}
	leaseModel := derivedLeaseModel
	if leaseModel == "" {
		leaseModel = cleanValue(agent.LeaseModel)
	}
	version := n.Version

	return canonicalCore{
		Normalized: n,
		AgentType:  agentType,
		Vendor:     vendor,
		Provider:   provider,
		AgentName:  agentName,
		LeaseModel: leaseModel,
		Version:    version,
	}
}

func NormalizeAgentIdentity(agent AgentIdentityLike) NormalizedAgentIdentity {
	provider := providerLabel(agent.Provider, agent.Command)
	version := cleanValue(agent.Version)
	flavor := cleanValue(agent.Flavor)
	rawModel := cleanValue(agent.Model)

	switch AgentProviderId(provider) {
	case ProviderOpenCode:
		parsed := ParseOpenCodePath(rawModel)
		fallbackVersion := parsed.Version
		if fallbackVersion == "" {
			fallbackVersion = version
		}
		result := NormalizedAgentIdentity{Provider: provider}
		if parsed.Model != "" {
			result.Model = parsed.Model
		} else if rawModel != "" {
			result.Model = rawModel
		}
		if fallbackVersion != "" {
			result.Version = fallbackVersion
		}
		return result
	case ProviderCopilot:
		normalized := normalizeCopilotModel(rawModel)
		return combineProviderResult(
			normalized.Provider,
			NormalizedAgentIdentity{
				Model:    normalized.Model,
				Flavor:   normalized.Flavor,
				Version:  normalized.Version,
			},
			NormalizedAgentIdentity{Flavor: flavor, Version: version},
		)
	case ProviderCodex:
		return combineProviderResult(
			provider,
			normalizeCodexModel(rawModel),
			NormalizedAgentIdentity{Flavor: flavor, Version: version},
		)
	case ProviderClaude:
		return combineProviderResult(
			provider,
			normalizeClaudeModel(rawModel),
			NormalizedAgentIdentity{Flavor: flavor, Version: version},
		)
	case ProviderGemini:
		return combineProviderResult(
			provider,
			normalizeGeminiModel(rawModel),
			NormalizedAgentIdentity{Flavor: flavor, Version: version},
		)
	default:
		result := NormalizedAgentIdentity{}
		if provider != "" {
			result.Provider = provider
		}
		if rawModel != "" {
			result.Model = rawModel
		}
		if flavor != "" {
			result.Flavor = flavor
		}
		if version != "" {
			result.Version = version
		}
		return result
	}
}

func combineProviderResult(provider string, parsed, fallback NormalizedAgentIdentity) NormalizedAgentIdentity {
	result := NormalizedAgentIdentity{Provider: provider}
	if parsed.Model != "" {
		result.Model = parsed.Model
	}
	flavor := parsed.Flavor
	if flavor == "" {
		flavor = fallback.Flavor
	}
	if flavor != "" {
		result.Flavor = flavor
	}
	version := parsed.Version
	if version == "" {
		version = fallback.Version
	}
	if version != "" {
		result.Version = version
	}
	return result
}

var codexFlavorDisplay = map[string]string{
	"codex-max":   "Codex Max",
	"codex-mini":  "Codex Mini",
	"codex-spark": "Codex Spark",
	"codex":       "Codex",
	"mini":        "Mini",
}

var miniWordRe = regexp.MustCompile(`\bmini\b`)

func normalizeCodexModel(rawModel string) NormalizedAgentIdentity {
	cleaned := strings.ToLower(cleanValue(rawModel))
	if cleaned == "" {
		return NormalizedAgentIdentity{}
	}
	if !strings.Contains(cleaned, "gpt") && !strings.Contains(cleaned, "chatgpt") && !strings.Contains(cleaned, "codex") {
		return NormalizedAgentIdentity{Model: strings.TrimSpace(rawModel)}
	}
	var modelName string
	if strings.Contains(cleaned, "chatgpt") {
		modelName = "ChatGPT"
	} else {
		modelName = "GPT"
	}
	versionRe := regexp.MustCompile(`(?:gpt|chatgpt)-?(\d+(?:\.\d+)*)`)
	versionMatch := versionRe.FindStringSubmatch(cleaned)
	var version string
	if len(versionMatch) >= 2 {
		version = versionMatch[1]
	}

	flavorKey := ""
	switch {
	case strings.Contains(cleaned, "codex-max"):
		flavorKey = "codex-max"
	case strings.Contains(cleaned, "codex-mini"):
		flavorKey = "codex-mini"
	case strings.Contains(cleaned, "codex-spark"):
		flavorKey = "codex-spark"
	case strings.Contains(cleaned, "codex"):
		flavorKey = "codex"
	case strings.Contains(cleaned, "codex"):
		flavorKey = "codex"
	case miniWordRe.MatchString(cleaned):
		flavorKey = "mini"
	}
	flavor := codexFlavorDisplay[flavorKey]

	result := NormalizedAgentIdentity{Model: modelName}
	if flavor != "" {
		result.Flavor = flavor
	}
	if version != "" {
		result.Version = version
	}
	return result
}

func claudeFlavorDisplay(family string, hasOneMillion, hasFast bool) string {
	head := strings.ToUpper(family[:1]) + strings.ToLower(family[1:])
	if hasOneMillion {
		return head + " (1M context)"
	}
	if hasFast {
		return head + " (Fast)"
	}
	return head
}

func normalizeClaudeModel(rawModel string) NormalizedAgentIdentity {
	cleaned := strings.ToLower(cleanValue(rawModel))
	if cleaned == "" {
		return NormalizedAgentIdentity{}
	}
	familyRe := regexp.MustCompile(`(opus|sonnet|haiku)`)
	familyMatch := familyRe.FindStringSubmatch(strings.ToLower(cleaned))

	hasOneMillion := strings.Contains(cleaned, "1m")
	hasFast := strings.Contains(cleaned, " fast") || strings.Contains(cleaned, "-fast")

	versionTarget := cleaned
	versionTarget = regexp.MustCompile(`-1m\b`).ReplaceAllString(versionTarget, "")
	versionTarget = regexp.MustCompile(`\b1m\b`).ReplaceAllString(versionTarget, "")
	versionTarget = regexp.MustCompile(`-fast\b`).ReplaceAllString(versionTarget, "")
	versionTarget = regexp.MustCompile(`\bfast\b`).ReplaceAllString(versionTarget, "")

	versionRe := regexp.MustCompile(`(?:opus|sonnet|haiku)[- ](\d+(?:[-.]\d+)*)`)
	versionMatch := versionRe.FindStringSubmatch(versionTarget)
	var normalizedVersion string
	if len(versionMatch) >= 2 {
		normalizedVersion = strings.ReplaceAll(versionMatch[1], "-", ".")
	}

	var flavor string
	if len(familyMatch) >= 2 {
		flavor = claudeFlavorDisplay(familyMatch[1], hasOneMillion, hasFast)
	}

	result := NormalizedAgentIdentity{}
	if len(familyMatch) >= 2 {
		result.Model = "Claude"
	}
	if flavor != "" {
		result.Flavor = flavor
	}
	if normalizedVersion != "" {
		result.Version = normalizedVersion
	}
	return result
}

func geminiFlavorDisplay(family string, preview bool) string {
	parts := strings.Split(family, "-")
	titleParts := make([]string, len(parts))
	for i, p := range parts {
		titleParts[i] = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
	}
	head := strings.Join(titleParts, " ")
	if preview {
		return head + " (Preview)"
	}
	return head
}

func normalizeGeminiModel(rawModel string) NormalizedAgentIdentity {
	cleaned := strings.ToLower(cleanValue(rawModel))
	if cleaned == "" {
		return NormalizedAgentIdentity{Model: "Gemini"}
	}
	versionRe := regexp.MustCompile(`gemini[- ](\d+(?:\.\d+)*)`)
	versionMatch := versionRe.FindStringSubmatch(cleaned)
	familyRe := regexp.MustCompile(`(pro|flash-lite|flash)(?:-(preview))?`)
	familyMatch := familyRe.FindStringSubmatch(cleaned)

	var flavor string
	if len(familyMatch) >= 2 {
		preview := len(familyMatch) >= 3 && familyMatch[2] != ""
		flavor = geminiFlavorDisplay(familyMatch[1], preview)
	}

	result := NormalizedAgentIdentity{Model: "Gemini"}
	if flavor != "" {
		result.Flavor = flavor
	}
	if len(versionMatch) >= 2 {
		result.Version = versionMatch[1]
	}
	return result
}

func normalizeCopilotModel(rawModel string) copilotNormalized {
	cleaned := strings.ToLower(cleanValue(rawModel))
	if cleaned == "" {
		return copilotNormalized{Provider: "Copilot"}
	}
	if strings.Contains(cleaned, "gpt") || strings.Contains(cleaned, "chatgpt") || strings.Contains(cleaned, "codex") {
		n := normalizeCodexModel(rawModel)
		return copilotNormalized{
			Provider: "Copilot",
			Model:    n.Model,
			Flavor:   n.Flavor,
			Version:  n.Version,
		}
	}
	if strings.Contains(cleaned, "gemini") {
		n := normalizeGeminiModel(rawModel)
		return copilotNormalized{
			Provider: "Copilot",
			Model:    n.Model,
			Flavor:   n.Flavor,
			Version:  n.Version,
		}
	}
	if strings.Contains(cleaned, "claude") || strings.Contains(cleaned, "opus") || strings.Contains(cleaned, "sonnet") || strings.Contains(cleaned, "haiku") {
		n := normalizeClaudeModel(rawModel)
		return copilotNormalized{
			Provider: "Copilot",
			Model:    n.Model,
			Flavor:   n.Flavor,
			Version:  n.Version,
		}
	}
	result := copilotNormalized{Provider: "Copilot"}
	if rawModel != "" {
		result.Model = strings.TrimSpace(rawModel)
	}
	return result
}

type copilotNormalized struct {
	Provider string
	Model    string
	Flavor   string
	Version  string
}

func FormatAgentDisplayLabel(agent AgentIdentityLike) string {
	n := NormalizeAgentIdentity(agent)
	label := formatAgentOptionLabel(AgentOptionSeed{
		Provider: n.Provider,
		Model:    n.Model,
		Flavor:   n.Flavor,
		Version:  n.Version,
	})
	if label != "" {
		return label
	}
	if v := cleanValue(agent.Label); v != "" {
		return v
	}
	if v := cleanValue(agent.Command); v != "" {
		return v
	}
	return "Unknown"
}

func FormatAgentFamily(option AgentOptionSeed) string {
	provider := cleanValue(option.Provider)
	model := cleanValue(option.Model)
	flavor := cleanValue(option.Flavor)
	sameAsProvider := func(s string) bool {
		return s != "" && provider != "" && strings.EqualFold(s, provider)
	}
	modelOut := model
	if sameAsProvider(model) {
		modelOut = ""
	}
	parts := []string{provider, modelOut, flavor}
	filtered := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			filtered = append(filtered, p)
		}
	}
	return strings.Join(filtered, " ")
}

func FormatLeaseModelDisplay(option AgentOptionSeed) string {
	provider := cleanValue(option.Provider)
	model := cleanValue(option.Model)
	flavor := cleanValue(option.Flavor)
	sameAsProvider := func(s string) bool {
		return s != "" && provider != "" && strings.EqualFold(s, provider)
	}
	modelOut := model
	if sameAsProvider(modelOut) {
		modelOut = ""
	}
	parts := []string{modelOut, flavor}
	filtered := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			filtered = append(filtered, p)
		}
	}
	result := strings.Join(filtered, " ")
	if result == "" {
		return ""
	}
	return result
}

func formatAgentOptionLabel(option AgentOptionSeed) string {
	family := FormatAgentFamily(option)
	version := cleanValue(option.Version)
	parts := []string{family}
	if version != "" {
		parts = append(parts, version)
	}
	return strings.Join(parts, " ")
}

func ParseAgentDisplayParts(agent AgentIdentityLike) AgentDisplayParts {
	providerID := DetectAgentProviderId(agent.Command)
	pills := []string{}

	if providerID == ProviderOpenCode {
		return openCodeDisplayParts(agent)
	}
	if providerID == ProviderCopilot {
		pills = append(pills, "copilot")
		pills = append(pills, "cli")
		return AgentDisplayParts{
			Label: FormatAgentDisplayLabel(agent),
			Pills: pills,
		}
	}
	pills = append(pills, "cli")
	return AgentDisplayParts{
		Label: FormatAgentDisplayLabel(agent),
		Pills: pills,
	}
}

func openCodeDisplayParts(agent AgentIdentityLike) AgentDisplayParts {
	rawModel := cleanValue(agent.Model)
	if rawModel == "" {
		return AgentDisplayParts{Label: "OpenCode", Pills: []string{"cli"}}
	}
	parsed := ParseOpenCodePath(rawModel)
	version := parsed.Version
	if version == "" {
		version = cleanValue(agent.Version)
	}
	parts := []string{"OpenCode", parsed.Model}
	if version != "" {
		parts = append(parts, version)
	}
	label := strings.Join(parts, " ")
	var pills []string
	if parsed.Router != "" {
		pills = append(pills, parsed.Router)
	} else if parsed.Vendor == "" {
		pills = append(pills, "opencode")
	}
	pills = append(pills, "cli")
	return AgentDisplayParts{Label: label, Pills: pills}
}

var commandDisplayLabels = map[string]string{
	"claude":       "Claude",
	"copilot":      "Copilot",
	"codex":        "Codex",
	"codex-cli":    "Codex",
	"gemini":       "Gemini",
	"opencode":     "OpenCode",
}

func displayCommandLabel(command string) string {
	lower := strings.ToLower(strings.TrimSpace(command))
	if lower == "" {
		return ""
	}
	if label, ok := commandDisplayLabels[lower]; ok {
		return label
	}
	for key, label := range commandDisplayLabels {
		if strings.Contains(lower, key) {
			return label
		}
	}
	return ""
}

type OpenCodePathParts struct {
	Model   string
	Version string
	Router  string
	Vendor  string
}

func ParseOpenCodePath(rawModel string) OpenCodePathParts {
	if rawModel == "" {
		return OpenCodePathParts{Model: ""}
	}
	tokens := strings.Split(rawModel, "/")
	filtered := make([]string, 0, len(tokens))
	for _, t := range tokens {
		if t != "" {
			filtered = append(filtered, t)
		}
	}
	tokens = filtered
	if len(tokens) == 0 {
		return OpenCodePathParts{Model: rawModel}
	}

	lastToken := tokens[len(tokens)-1]
	split := SplitOpenCodeModelToken(lastToken)

	if len(tokens) >= 3 {
		return composePathParts(tokens[0], tokens[len(tokens)-2], split)
	}
	if len(tokens) == 2 {
		return composePathParts("", tokens[0], split)
	}
	return composePathParts("", "", split)
}

func composePathParts(router, vendor string, split OpenCodeModelSplit) OpenCodePathParts {
	segments := []string{}
	if router != "" {
		segments = append(segments, formatOpenCodeSegment(router))
	}
	if vendor != "" {
		segments = append(segments, formatOpenCodeSegment(vendor))
	}
	if split.Name != "" {
		segments = append(segments, split.Name)
	}
	if split.Tail != "" {
		segments = append(segments, split.Tail)
	}
	result := OpenCodePathParts{
		Model: strings.Join(segments, " "),
	}
	if split.Version != "" {
		result.Version = split.Version
	}
	if router != "" {
		result.Router = strings.ToLower(router)
	}
	if vendor != "" {
		result.Vendor = strings.ToLower(vendor)
	}
	return result
}

type OpenCodeModelSplit struct {
	Name    string
	Version string
	Tail    string
}

func SplitOpenCodeModelToken(token string) OpenCodeModelSplit {
	if token == "" {
		return OpenCodeModelSplit{}
	}
	segments := strings.Split(token, "-")
	firstVersionIdx := -1
	trailingNumRe := regexp.MustCompile(`(\d+(?:\.\d+)*)$`)
	for i, s := range segments {
		if trailingNumRe.MatchString(s) {
			firstVersionIdx = i
			break
		}
	}
	if firstVersionIdx < 0 {
		return OpenCodeModelSplit{Name: formatModelName(token)}
	}
	vt := collectVersionTokens(segments, firstVersionIdx)
	if len(vt.Values) == 0 {
		return OpenCodeModelSplit{Name: formatModelName(token)}
	}
	nameParts := segments[:firstVersionIdx]
	if vt.FirstSegmentPrefix != "" {
		nameParts = append(nameParts, vt.FirstSegmentPrefix)
	}
	tailSegments := segments[vt.AfterIdx:]
	result := OpenCodeModelSplit{
		Name:    formatModelName(strings.Join(nameParts, "-")),
		Version: strings.Join(vt.Values, "."),
	}
	if len(tailSegments) > 0 {
		formattedTails := make([]string, len(tailSegments))
		for i, s := range tailSegments {
			formattedTails[i] = formatOpenCodeSegment(s)
		}
		result.Tail = strings.Join(formattedTails, " ")
	}
	return result
}

type versionTokens struct {
	Values               []string
	FirstSegmentPrefix   string
	AfterIdx             int
}

func collectVersionTokens(segments []string, startIdx int) versionTokens {
	values := []string{}
	first := segments[startIdx]
	numRe := regexp.MustCompile(`^(.*?)(\d+(?:\.\d+)*)$`)
	m := numRe.FindStringSubmatch(first)
	if m == nil {
		return versionTokens{AfterIdx: startIdx}
	}
	prefix := m[1]
	values = append(values, m[2])
	i := startIdx + 1
	pureNumRe := regexp.MustCompile(`^\d+(?:\.\d+)*$`)
	for i < len(segments) && pureNumRe.MatchString(segments[i]) {
		values = append(values, segments[i])
		i++
	}
	return versionTokens{
		Values:               values,
		FirstSegmentPrefix:   prefix,
		AfterIdx:             i,
	}
}

var vendorDisplayNames = map[string]string{
	"openrouter": "OpenRouter",
	"moonshotai": "MoonshotAI",
	"anthropic":   "Anthropic",
	"z-ai":        "Z-AI",
	"mistral":     "Mistral",
	"mistralai":   "MistralAI",
	"google":      "Google",
	"copilot":     "Copilot",
	"opencode":    "OpenCode",
	"openai":      "OpenAI",
	"meta":        "Meta",
	"qwen":        "Qwen",
	"deepseek":    "DeepSeek",
	"xai":         "xAI",
	"perplexity":  "Perplexity",
	"cohere":      "Cohere",
	"glm":         "GLM",
	"llama":       "Llama",
	"bert":        "BERT",
	"rwkv":        "RWKV",
	"t5":          "T5",
}

var trailingUpperSuffixes = []string{"ai", "ml", "io", "js"}

func formatOpenCodeSegment(token string) string {
	if token == "" {
		return token
	}
	if label, ok := vendorDisplayNames[strings.ToLower(token)]; ok {
		return label
	}
	return capitalizeWithSuffix(token)
}

func capitalizeWithSuffix(token string) string {
	if token == "" {
		return token
	}
	lower := strings.ToLower(token)
	for _, suffix := range trailingUpperSuffixes {
		if len(lower) > len(suffix) && strings.HasSuffix(lower, suffix) {
			stem := lower[:len(lower)-len(suffix)]
			return capitalizeFirst(stem) + strings.ToUpper(suffix)
		}
	}
	return capitalizeFirst(lower)
}

func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func formatModelName(name string) string {
	if name == "" {
		return name
	}
	parts := strings.Split(name, "-")
	head := formatOpenCodeSegment(parts[0])
	if len(parts) == 1 {
		return head
	}
	result := head
	for i := 1; i < len(parts); i++ {
		part := parts[i]
		if len(part) == 1 {
			result += "-" + strings.ToLower(part)
		} else {
			result += " " + formatOpenCodeSegment(part)
		}
	}
	return result
}

func BuildAgentOptionID(agentID string, option AgentOptionSeed) string {
	modelID := cleanValue(option.ModelID)
	if modelID != "" {
		lower := strings.ToLower(modelID)
		return agentID + "-" + sanitizeOptionID(lower)
	}
	parts := []string{agentID}
	if m := cleanValue(option.Model); m != "" {
		parts = append(parts, sanitizeOptionID(strings.ToLower(m)))
	}
	if f := cleanValue(option.Flavor); f != "" {
		parts = append(parts, sanitizeOptionID(strings.ToLower(f)))
	}
	if v := cleanValue(option.Version); v != "" {
		parts = append(parts, sanitizeOptionID(strings.ToLower(v)))
	}
	return strings.Join(parts, "-")
}

func sanitizeOptionID(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

func ToCanonicalLeaseIdentitySimple(command, model, provider, flavor, version string) CanonicalLeaseIdentity {
	return ToCanonicalLeaseIdentity(AgentIdentityLike{
		Command:  command,
		Model:    model,
		Provider: provider,
		Flavor:   flavor,
		Version:  version,
	})
}

// OpenCodeModelSelection holds the result of splitting an OpenCode model
// identifier into its provider prefix and the remainder model path.
type OpenCodeModelSelection struct {
	ProviderID string
	ModelID   string
}

// ParseOpenCodeModelSelection splits an OpenCode model identifier of the form
// "<providerID>/<modelID>" into its two components. The modelID may itself
// contain additional slashes (e.g. "openrouter/z-ai/glm-5.1" → providerID
// "openrouter", modelID "z-ai/glm-5.1").
//
// Returns nil for empty or whitespace-only input. Returns an error if the
// input lacks a provider slash, per the transport contract that requires a
// loud failure rather than a silent fallback.
func ParseOpenCodeModelSelection(model string) (*OpenCodeModelSelection, error) {
	trimmed := strings.TrimSpace(model)
	if trimmed == "" {
		return nil, nil
	}
	sep := strings.Index(trimmed, "/")
	if sep <= 0 || sep == len(trimmed)-1 {
		return nil, fmt.Errorf("FOOLERY DISPATCH FAILURE: invalid OpenCode model %q; expected <providerID>/<modelID>", trimmed)
	}
	return &OpenCodeModelSelection{
		ProviderID: trimmed[:sep],
		ModelID:    trimmed[sep+1:],
	}, nil
}

func cleanValue(s string) string {
	t := strings.TrimSpace(s)
	if t == "" {
		return ""
	}
	return t
}