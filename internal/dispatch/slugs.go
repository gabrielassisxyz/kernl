package dispatch

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

const OrchestrationWaveLabel = "orchestration:wave"
const OrchestrationWaveLabelPrefix = OrchestrationWaveLabel + ":"

var actorLastNames = []string{
	"streep", "washington", "freeman", "depp", "blanchett", "winslet",
	"ledger", "pacino", "hoffman", "hanks", "daylewis", "swank",
	"bardem", "theron", "pitt", "jolie", "waltz", "weaver",
	"croft", "foster", "reeves", "clooney", "adams", "redmayne",
	"poitier", "mckellen", "affleck", "hamill", "fonda", "eastwood",
}

var movieTitleWords = []string{
	"arrival", "gravity", "matrix", "heat", "memento", "casablanca",
	"vertigo", "sunset", "godfather", "noir", "jaws", "fargo",
	"inception", "apollo", "amadeus", "gladiator", "spotlight",
	"parasite", "goodfellas", "moonlight", "interstellar", "prestige",
	"whiplash", "network", "rocky", "titanic", "birdman", "uncut",
	"arrival", "encore",
}

var setBuzzwords = []string{
	"gaffer", "slate", "take", "rushes", "dailies", "blocking",
	"callback", "table", "location", "stunt", "foley", "grip",
	"boom", "lens", "dolly", "chroma", "wardrobe", "props",
	"montage", "cutaway", "continuity", "scene", "rehearsal",
	"premiere", "screening", "voiceover",
}

var slugifyRe1 = regexp.MustCompile(`[^a-z0-9]+`)
var slugifyRe2 = regexp.MustCompile(`^-+|-+$`)
var slugifyRe3 = regexp.MustCompile(`--+`)
var numericRe = regexp.MustCompile(`^\d+$`)
var waveScenePrefixRe = regexp.MustCompile(`(?i)^(?:wave|scene)\s+[^:]+:\s*`)

func slugify(value string) string {
	s := strings.ToLower(strings.TrimSpace(value))
	s = slugifyRe1.ReplaceAllString(s, "-")
	s = slugifyRe3.ReplaceAllString(s, "-")
	s = slugifyRe2.ReplaceAllString(s, "")
	return s
}

func NormalizeWaveSlugCandidate(value string) string {
	return slugify(value)
}

func IsWaveLabel(label string) bool {
	return label == OrchestrationWaveLabel || strings.HasPrefix(label, OrchestrationWaveLabelPrefix)
}

func IsInternalLabel(label string) bool {
	return IsWaveLabel(label) || strings.HasPrefix(label, "stage:")
}

func IsReadOnlyLabel(label string) bool {
	return strings.HasPrefix(label, "attempts:")
}

func IsWaveSlugLabel(label string) bool {
	if !strings.HasPrefix(label, OrchestrationWaveLabelPrefix) {
		return false
	}
	slug := strings.TrimPrefix(label, OrchestrationWaveLabelPrefix)
	return len(slug) > 0
}

func GetWaveSlugLabels(labels []string) []string {
	var result []string
	for _, l := range labels {
		if IsWaveSlugLabel(l) {
			result = append(result, l)
		}
	}
	return result
}

// ExtractWaveSlug returns the slug portion from the first label matching
// "orchestration:wave:<slug>". Returns empty string if not found.
func ExtractWaveSlug(labels []string) string {
	for _, l := range GetWaveSlugLabels(labels) {
		raw := strings.TrimPrefix(l, OrchestrationWaveLabelPrefix)
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		normalized := slugify(raw)
		if normalized != "" {
			return normalized
		}
	}
	return ""
}

func IsLegacyNumericWaveSlug(slug string) bool {
	if slug == "" {
		return false
	}
	return numericRe.MatchString(slug)
}

func BuildWaveSlugLabel(slug string) string {
	return OrchestrationWaveLabelPrefix + slugify(slug)
}

func composedCandidate(seed, attempt int) string {
	actor := actorLastNames[(seed+attempt)%len(actorLastNames)]
	movie := movieTitleWords[(seed*3+attempt)%len(movieTitleWords)]
	buzz := setBuzzwords[(seed*7+attempt)%len(setBuzzwords)]
	variant := attempt % 3
	if variant == 0 {
		return actor + "-" + movie
	}
	if variant == 1 {
		return movie + "-" + buzz
	}
	return actor + "-" + buzz
}

// AllocateWaveSlug returns a unique slug, preferring preferredSlug if available
// and not already in usedSet. Adds the returned slug to usedSet.
func AllocateWaveSlug(usedSet map[string]bool, preferredSlug string) string {
	preferred := slugify(preferredSlug)
	if preferred != "" && !usedSet[preferred] {
		usedSet[preferred] = true
		return preferred
	}

	seed := int(time.Now().UnixMilli()) + len(usedSet)*17
	maxAttempts := len(actorLastNames) * len(movieTitleWords) * len(setBuzzwords)
	for attempt := 0; attempt < maxAttempts; attempt++ {
		candidate := composedCandidate(seed, attempt)
		if usedSet[candidate] {
			continue
		}
		usedSet[candidate] = true
		return candidate
	}

	fallbackBase := composedCandidate(seed, maxAttempts)
	for suffix := 2; suffix < 10000; suffix++ {
		candidate := fmt.Sprintf("%s-%d", fallbackBase, suffix)
		if usedSet[candidate] {
			continue
		}
		usedSet[candidate] = true
		return candidate
	}

	emergency := fmt.Sprintf("%s-%d", fallbackBase, time.Now().UnixMilli())
	usedSet[emergency] = true
	return emergency
}

func BuildWaveTitle(slug, name string) string {
	cleanName := strings.TrimSpace(name)
	if cleanName == "" {
		return "Scene " + slug
	}
	return "Scene " + slug + ": " + cleanName
}

func RewriteWaveTitleSlug(title, slug string) string {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return "Scene " + slug
	}
	if waveScenePrefixRe.MatchString(trimmed) {
		return waveScenePrefixRe.ReplaceAllString(trimmed, "Scene "+slug+": ")
	}
	return "Scene " + slug + ": " + trimmed
}
