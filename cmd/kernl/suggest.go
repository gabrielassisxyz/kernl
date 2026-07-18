package main

import "fmt"

// damerauDistance is the restricted Damerau-Levenshtein (optimal string
// alignment) distance. Plain Levenshtein misses transpositions, which are a
// common class of real typos (e.g. "autnoomous" -> "autonomous" is 1 swap).
func damerauDistance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	d := make([][]int, la+1)
	for i := range d {
		d[i] = make([]int, lb+1)
		d[i][0] = i
	}
	for j := 0; j <= lb; j++ {
		d[0][j] = j
	}
	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			del, ins, sub := d[i-1][j]+1, d[i][j-1]+1, d[i-1][j-1]+cost
			m := del
			if ins < m {
				m = ins
			}
			if sub < m {
				m = sub
			}
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				if t := d[i-2][j-2] + 1; t < m {
					m = t
				}
			}
			d[i][j] = m
		}
	}
	return d[la][lb]
}

// suggest returns the closest candidate to input, or "" when nothing is close
// enough to state with confidence. Distance 1 always qualifies; distance 2
// only for longer inputs (>= 5 runes) and only when the best match is unique.
func suggest(input string, candidates []string) string {
	best, bestDist, ties := "", 3, 0
	for _, c := range candidates {
		dist := damerauDistance(input, c)
		switch {
		case dist < bestDist:
			best, bestDist, ties = c, dist, 1
		case dist == bestDist:
			ties++
		}
	}
	if bestDist == 1 || (bestDist == 2 && len([]rune(input)) >= 5 && ties == 1) {
		return best
	}
	return ""
}

// didYouMean renders the standard hint fragment, or "" when no candidate is
// close enough.
func didYouMean(input string, candidates []string) string {
	if s := suggest(input, candidates); s != "" {
		return fmt.Sprintf(" — did you mean %q?", s)
	}
	return ""
}
