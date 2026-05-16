package workflow

import (
	"regexp"
	"strings"
)

var metadataLineRE = regexp.MustCompile(`(?im)^([a-zA-Z0-9_]+):\s*(.*?)\s*$`)

func stripBOM(s string) string {
	return strings.TrimPrefix(s, "\ufeff")
}

func GetMetadataField(desc, key string) string {
	desc = stripBOM(desc)
	keyLower := strings.ToLower(key)
	for _, line := range strings.Split(desc, "\n") {
		m := metadataLineRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if strings.ToLower(m[1]) == keyLower {
			return strings.TrimSpace(m[2])
		}
	}
	return ""
}

func AddMetadataField(desc, key, value string) string {
	desc = stripBOM(desc)
	keyLower := strings.ToLower(key)
	var b strings.Builder
	replaced := false
	for i, line := range strings.Split(desc, "\n") {
		m := metadataLineRE.FindStringSubmatch(line)
		if m != nil && strings.ToLower(m[1]) == keyLower {
			if !replaced {
				if i > 0 {
					b.WriteString("\n")
				}
				b.WriteString(key + ": " + value)
				replaced = true
			}
			continue
		}
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(line)
	}
	if !replaced {
		if b.Len() > 0 && !strings.HasSuffix(b.String(), "\n") {
			b.WriteString("\n")
		}
		b.WriteString(key + ": " + value + "\n")
	}
	return b.String()
}

func GetWorktreePath(d string) string       { return GetMetadataField(d, "worktree_path") }
func SetWorktreePath(d, v string) string    { return AddMetadataField(d, "worktree_path", v) }
func GetWorktreeBranch(d string) string     { return GetMetadataField(d, "worktree_branch") }
func SetWorktreeBranch(d, v string) string  { return AddMetadataField(d, "worktree_branch", v) }
func GetEpicBranch(d string) string         { return GetMetadataField(d, "epic_branch") }
func SetEpicBranch(d, v string) string      { return AddMetadataField(d, "epic_branch", v) }
func GetPRURL(d string) string              { return GetMetadataField(d, "pr_url") }
func SetPRURL(d, v string) string           { return AddMetadataField(d, "pr_url", v) }
func GetMergeConflictAt(d string) string    { return GetMetadataField(d, "merge_conflict_at") }
func SetMergeConflictAt(d, v string) string { return AddMetadataField(d, "merge_conflict_at", v) }
func GetMergeOutcome(d string) string       { return GetMetadataField(d, "merge_outcome") }
func SetMergeOutcome(d, v string) string    { return AddMetadataField(d, "merge_outcome", v) }

func RemoveMetadataField(desc, key string) string {
	desc = stripBOM(desc)
	keyLower := strings.ToLower(key)
	var b strings.Builder
	wrote := false
	for _, line := range strings.Split(desc, "\n") {
		m := metadataLineRE.FindStringSubmatch(line)
		if m != nil && strings.ToLower(m[1]) == keyLower {
			continue
		}
		if wrote {
			b.WriteString("\n")
		}
		b.WriteString(line)
		wrote = true
	}
	return b.String()
}
