package ui

import (
	"sort"
	"strings"
	"unicode"
)

// fuzzyScore reports whether every rune of term appears in s in order
// (case-insensitive) and how good the match is. Consecutive runs, word starts
// and a plain substring hit score higher; gaps cost a little.
func fuzzyScore(term, s string) (int, bool) {
	t := []rune(strings.ToLower(term))
	if len(t) == 0 {
		return 0, true
	}
	orig := []rune(s)
	low := []rune(strings.ToLower(s))
	if len(low) != len(orig) { // odd case-folding changed the length; fall back to raw runes
		low = orig
	}
	score, ti, prev, first := 0, 0, -2, -1
	for i, r := range low {
		if ti >= len(t) {
			break
		}
		if r != t[ti] {
			continue
		}
		score += 10
		if i == prev+1 {
			score += 15
		}
		if i == 0 || !unicode.IsLetter(orig[i-1]) && !unicode.IsDigit(orig[i-1]) ||
			unicode.IsUpper(orig[i]) && unicode.IsLower(orig[i-1]) {
			score += 20
		}
		if prev >= 0 {
			score -= min(i-prev-1, 5)
		}
		if first < 0 {
			first = i
		}
		prev = i
		ti++
	}
	if ti < len(t) {
		return 0, false
	}
	if first == 0 {
		score += 30 // anchored at the start
	}
	if strings.Contains(strings.ToLower(s), strings.ToLower(term)) {
		score += 50
	}
	return score, true
}

// rankFilter applies the list-pane filter: fuzzy subsequence match on any
// field, best matches first (ties keep their order). A leading `!` inverts
// the match and keeps the original order. The first field is the primary one;
// matches only in later fields score half so display names win.
func rankFilter[T any](items []T, filter string, fields func(T) []string) []T {
	if filter == "" {
		return items
	}
	invert := strings.HasPrefix(filter, "!")
	term := strings.TrimPrefix(filter, "!")
	if term == "" {
		return items
	}
	type scored struct {
		v T
		s int
	}
	var hits []scored
	for _, it := range items {
		best, ok := 0, false
		for i, f := range fields(it) {
			if sc, m := fuzzyScore(term, f); m {
				if i > 0 {
					sc /= 2
				}
				if !ok || sc > best {
					best = sc
				}
				ok = true
			}
		}
		if ok != invert {
			hits = append(hits, scored{it, best})
		}
	}
	if !invert {
		sort.SliceStable(hits, func(i, j int) bool { return hits[i].s > hits[j].s })
	}
	out := make([]T, len(hits))
	for i, h := range hits {
		out[i] = h.v
	}
	return out
}
