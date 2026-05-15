package backend

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type MemoryManagerType string

const (
	MemoryManagerKnots MemoryManagerType = "knots"
	MemoryManagerBeads MemoryManagerType = "beads"
)

type memoryManagerImpl struct {
	Type            MemoryManagerType
	Label           string
	MarkerDirectory string
	Precedence      int
}

var knownMemoryManagers = []memoryManagerImpl{
	{Type: MemoryManagerKnots, Label: "Knots", MarkerDirectory: ".knots", Precedence: 0},
	{Type: MemoryManagerBeads, Label: "Beads", MarkerDirectory: ".beads", Precedence: 1},
}

func IsKnownMemoryManagerType(value string) bool {
	if value == "" {
		return false
	}
	for _, mm := range knownMemoryManagers {
		if string(mm.Type) == value {
			return true
		}
	}
	return false
}

func KnownMemoryManagerMarkers() []string {
	markers := make([]string, len(knownMemoryManagers))
	for i, mm := range knownMemoryManagers {
		markers[i] = mm.MarkerDirectory
	}
	return markers
}

func DetectMemoryManager(repoPath string) MemoryManagerType {
	for _, mm := range knownMemoryManagers {
		if _, err := os.Stat(filepath.Join(repoPath, mm.MarkerDirectory)); err == nil {
			return mm.Type
		}
	}
	return MemoryManagerBeads
}

type RegistryRepo struct {
	Path               string            `json:"path"`
	Name               string            `json:"name"`
	AddedAt            string            `json:"addedAt"`
	MemoryManagerType  MemoryManagerType `json:"memoryManagerType,omitempty"`
}

type Registry struct {
	Repos []RegistryRepo `json:"repos"`
}

type RepoMemoryManagerAuditResult struct {
	MissingRepoPaths []string
	FileMissing      bool
	Error            string
}

type RepoMemoryManagerBackfillResult struct {
	Changed           bool
	MigratedRepoPaths []string
	FileMissing       bool
	Error             string
}

type RegistryPermissionsAudit struct {
	FileMissing bool
	NeedsFix    bool
	ActualMode  *uint32
	Error       string
}

type RegistryPermissionsFixResult struct {
	FileMissing bool
	NeedsFix    bool
	ActualMode  *uint32
	Changed     bool
	Error       string
}

type RepoMemoryManagerSyncResult struct {
	Changed               bool
	FileMissing           bool
	RepoFound             bool
	PreviousMemoryManagerType MemoryManagerType
	MemoryManagerType     MemoryManagerType
	Error                 string
}

var registryFilePath = func() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "foolery", "registry.json")
}

var configDir = func() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "foolery")
}

func defaultMemoryManagerType(repoPath string) MemoryManagerType {
	for _, mm := range knownMemoryManagers {
		if _, err := os.Stat(filepath.Join(repoPath, mm.MarkerDirectory)); err == nil {
			return mm.Type
		}
	}
	return MemoryManagerBeads
}

func normalizeRepo(raw map[string]any) *RegistryRepo {
	pathVal, _ := raw["path"].(string)
	if pathVal == "" {
		return nil
	}

	name := ""
	if n, ok := raw["name"].(string); ok && n != "" {
		name = n
	} else {
		name = filepath.Base(pathVal)
	}

	addedAt := ""
	if a, ok := raw["addedAt"].(string); ok && a != "" {
		addedAt = a
	} else {
		addedAt = time.Unix(0, 0).UTC().Format(time.RFC3339)
	}

	var mmType MemoryManagerType
	if configured, ok := raw["memoryManagerType"].(string); ok && configured != "" {
		if IsKnownMemoryManagerType(configured) {
			mmType = MemoryManagerType(configured)
		} else {
			mmType = defaultMemoryManagerType(pathVal)
		}
	} else {
		mmType = defaultMemoryManagerType(pathVal)
	}

	return &RegistryRepo{
		Path:              pathVal,
		Name:              name,
		AddedAt:           addedAt,
		MemoryManagerType: mmType,
	}
}

func normalizeRegistry(raw map[string]any) Registry {
	reposSlice, ok := raw["repos"].([]any)
	if !ok {
		return Registry{Repos: nil}
	}

	var repos []RegistryRepo
	for _, item := range reposSlice {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		repo := normalizeRepo(entry)
		if repo != nil {
			repos = append(repos, *repo)
		}
	}
	return Registry{Repos: repos}
}

func readRawRegistry() (parsed map[string]any, fileMissing bool, errStr string) {
	path := registryFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, true, ""
		}
		return map[string]any{}, false, err.Error()
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return map[string]any{}, false, err.Error()
	}
	return raw, false, ""
}

func LoadRegistry() (Registry, error) {
	path := registryFilePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Registry{Repos: nil}, nil
		}
		return Registry{Repos: nil}, err
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return Registry{Repos: nil}, nil
	}
	return normalizeRegistry(raw), nil
}

func SaveRegistry(reg Registry) error {
	dir := configDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("FOOLERY DISPATCH FAILURE: creating registry dir %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(reg, "", "  ")
	if err != nil {
		return fmt.Errorf("FOOLERY DISPATCH FAILURE: marshaling registry: %w", err)
	}

	path := registryFilePath()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("FOOLERY DISPATCH FAILURE: writing registry %s: %w", path, err)
	}
	return nil
}

func AddRepo(repoPath string) (*RegistryRepo, error) {
	mm := DetectMemoryManager(repoPath)
	if mm == "" {
		mm = MemoryManagerBeads
	}

	reg, _ := LoadRegistry()
	for _, r := range reg.Repos {
		if r.Path == repoPath {
			return nil, fmt.Errorf("Repository already registered: %s", repoPath)
		}
	}

	repo := RegistryRepo{
		Path:              repoPath,
		Name:              filepath.Base(repoPath),
		AddedAt:           time.Now().UTC().Format(time.RFC3339),
		MemoryManagerType: mm,
	}
	reg.Repos = append(reg.Repos, repo)
	if err := SaveRegistry(reg); err != nil {
		return nil, err
	}
	return &repo, nil
}

func RemoveRepo(repoPath string) error {
	reg, _ := LoadRegistry()
	filtered := make([]RegistryRepo, 0, len(reg.Repos))
	for _, r := range reg.Repos {
		if r.Path != repoPath {
			filtered = append(filtered, r)
		}
	}
	reg.Repos = filtered
	return SaveRegistry(reg)
}

func ListRepos() ([]RegistryRepo, error) {
	reg, err := LoadRegistry()
	if err != nil {
		return nil, err
	}
	return reg.Repos, nil
}

func InspectMissingRepoMemoryManagerTypes() RepoMemoryManagerAuditResult {
	raw, fileMissing, errStr := readRawRegistry()
	if errStr != "" {
		return RepoMemoryManagerAuditResult{FileMissing: fileMissing, Error: errStr}
	}

	var missing []string
	reposSlice, ok := raw["repos"].([]any)
	if !ok {
		return RepoMemoryManagerAuditResult{MissingRepoPaths: []string{}, FileMissing: fileMissing}
	}

	for _, item := range reposSlice {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		pathVal, _ := entry["path"].(string)
		if pathVal == "" {
			continue
		}
		mm, _ := entry["memoryManagerType"].(string)
		if mm == "" {
			missing = append(missing, pathVal)
		}
	}

	return RepoMemoryManagerAuditResult{
		MissingRepoPaths: missing,
		FileMissing:      fileMissing,
	}
}

func BackfillMissingRepoMemoryManagerTypes() RepoMemoryManagerBackfillResult {
	raw, fileMissing, errStr := readRawRegistry()
	if errStr != "" {
		return RepoMemoryManagerBackfillResult{FileMissing: fileMissing, Error: errStr}
	}

	if fileMissing {
		return RepoMemoryManagerBackfillResult{FileMissing: true}
	}

	if raw == nil {
		return RepoMemoryManagerBackfillResult{}
	}

	reposSlice, ok := raw["repos"].([]any)
	if !ok {
		return RepoMemoryManagerBackfillResult{}
	}

	var migrated []string
	changed := false
	updated := make([]any, len(reposSlice))

	for i, item := range reposSlice {
		entry, ok := item.(map[string]any)
		if !ok {
			updated[i] = item
			continue
		}

		pathVal, _ := entry["path"].(string)
		if pathVal == "" {
			updated[i] = item
			continue
		}

		mm, _ := entry["memoryManagerType"].(string)
		if mm != "" {
			updated[i] = item
			continue
		}

		inferred := string(defaultMemoryManagerType(pathVal))
		newEntry := make(map[string]any)
		for k, v := range entry {
			newEntry[k] = v
		}
		newEntry["memoryManagerType"] = inferred
		updated[i] = newEntry
		migrated = append(migrated, pathVal)
		changed = true
	}

	if !changed {
		return RepoMemoryManagerBackfillResult{}
	}

	raw["repos"] = updated
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return RepoMemoryManagerBackfillResult{Error: err.Error()}
	}

	dir := configDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return RepoMemoryManagerBackfillResult{Error: err.Error()}
	}

	path := registryFilePath()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return RepoMemoryManagerBackfillResult{Error: err.Error()}
	}

	return RepoMemoryManagerBackfillResult{
		Changed:           true,
		MigratedRepoPaths: migrated,
	}
}

func normalizeMode(mode os.FileMode) uint32 {
	return uint32(mode.Perm())
}

func InspectRegistryPermissions() RegistryPermissionsAudit {
	path := registryFilePath()
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return RegistryPermissionsAudit{FileMissing: true, NeedsFix: false}
		}
		return RegistryPermissionsAudit{Error: err.Error()}
	}

	actual := normalizeMode(info.Mode())
	return RegistryPermissionsAudit{
		FileMissing: false,
		NeedsFix:    actual != 0o600,
		ActualMode:  &actual,
	}
}

func EnsureRegistryPermissions() RegistryPermissionsFixResult {
	audit := InspectRegistryPermissions()
	if audit.Error != "" || audit.FileMissing || !audit.NeedsFix {
		return RegistryPermissionsFixResult{
			FileMissing: audit.FileMissing,
			NeedsFix:    audit.NeedsFix,
			ActualMode:  audit.ActualMode,
			Changed:     false,
			Error:       audit.Error,
		}
	}

	path := registryFilePath()
	if err := os.Chmod(path, 0o600); err != nil {
		return RegistryPermissionsFixResult{
			NeedsFix:   true,
			ActualMode: audit.ActualMode,
			Error:      err.Error(),
		}
	}

	mode := uint32(0o600)
	return RegistryPermissionsFixResult{
		FileMissing: false,
		NeedsFix:    false,
		ActualMode:  &mode,
		Changed:     true,
	}
}

func UpdateRegisteredRepoMemoryManagerType(repoPath string, mmType MemoryManagerType) RepoMemoryManagerSyncResult {
	raw, fileMissing, errStr := readRawRegistry()
	if errStr != "" {
		return RepoMemoryManagerSyncResult{FileMissing: fileMissing, Error: errStr}
	}

	if fileMissing {
		return RepoMemoryManagerSyncResult{FileMissing: true}
	}

	reposSlice, ok := raw["repos"].([]any)
	if !ok {
		return RepoMemoryManagerSyncResult{}
	}

	repoFound := false
	var previousType MemoryManagerType
	changed := false

	updated := make([]any, len(reposSlice))
	for i, item := range reposSlice {
		entry, ok := item.(map[string]any)
		if !ok {
			updated[i] = item
			continue
		}

		pathVal, _ := entry["path"].(string)
		if pathVal != repoPath {
			updated[i] = item
			continue
		}

		repoFound = true
		existingMM, _ := entry["memoryManagerType"].(string)
		if IsKnownMemoryManagerType(existingMM) {
			previousType = MemoryManagerType(existingMM)
		}

		if previousType == mmType {
			updated[i] = item
			continue
		}

		newEntry := make(map[string]any)
		for k, v := range entry {
			newEntry[k] = v
		}
		newEntry["memoryManagerType"] = string(mmType)
		updated[i] = newEntry
		changed = true
	}

	if !repoFound {
		return RepoMemoryManagerSyncResult{
			RepoFound:         false,
			MemoryManagerType: mmType,
		}
	}

	if !changed {
		return RepoMemoryManagerSyncResult{
			RepoFound:                 true,
			PreviousMemoryManagerType: previousType,
			MemoryManagerType:         mmType,
		}
	}

	raw["repos"] = updated
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		return RepoMemoryManagerSyncResult{Error: err.Error()}
	}

	dir := configDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return RepoMemoryManagerSyncResult{Error: err.Error()}
	}

	path := registryFilePath()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return RepoMemoryManagerSyncResult{Error: err.Error()}
	}

	return RepoMemoryManagerSyncResult{
		Changed:                   true,
		RepoFound:                 true,
		PreviousMemoryManagerType: previousType,
		MemoryManagerType:         mmType,
	}
}

func extractBaseName(path string) string {
	if path == "" {
		return ""
	}
	path = strings.TrimRight(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return ""
	}
	if parts[len(parts)-1] == "" && len(parts) > 1 {
		return parts[len(parts)-2]
	}
	return parts[len(parts)-1]
}