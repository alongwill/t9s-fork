package ui

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// logFilterMode is how the filter text is read.
type logFilterMode int

const (
	filterWords logFilterMode = iota // every word present, `!word` absent
	filterFuzzy                      // `-f term`: subsequence match above a minimum score
	filterRegex                      // `-r regex`
)

// fuzzyMinPerRune is the score each rune of a `-f` term must average for a
// line to stay. Scattered letters, even at word starts, score less than that;
// a contiguous run (a substring hit) clears it comfortably.
const fuzzyMinPerRune = 30

// logFilter is a parsed log filter. The zero value matches every line.
type logFilter struct {
	mode    logFilterMode
	pos     []string // lower-cased words that must appear
	neg     []string // lower-cased words that must not appear
	fuzzy   string   // -f term
	fuzzyNg bool     // -f !term
	re      *regexp.Regexp
	invalid bool // -r with a regex that does not compile (yet): shows everything
	set     bool
}

// parseLogFilter reads the filter grammar:
//
//	dns timeout     every word, any order, case-insensitive
//	error !probe    `!word` must not appear
//	-f term         fuzzy (subsequence) match
//	-r regex        regular expression (case-insensitive)
func parseLogFilter(s string) logFilter {
	s = strings.TrimSpace(s)
	var f logFilter
	switch {
	case s == "-f" || s == "-r":
		return f
	case strings.HasPrefix(s, "-f "):
		t := strings.TrimSpace(s[3:])
		if t == "" {
			return f
		}
		f.mode, f.set = filterFuzzy, true
		if strings.HasPrefix(t, "!") && len(t) > 1 {
			f.fuzzyNg, t = true, t[1:]
		}
		f.fuzzy = t
	case strings.HasPrefix(s, "-r "):
		t := strings.TrimSpace(s[3:])
		if t == "" {
			return f
		}
		f.mode, f.set = filterRegex, true
		re, err := regexp.Compile("(?i)" + t)
		if err != nil {
			f.invalid = true
		} else {
			f.re = re
		}
	default:
		for _, w := range strings.Fields(strings.ToLower(s)) {
			if strings.HasPrefix(w, "!") {
				if len(w) > 1 {
					f.neg = append(f.neg, w[1:])
				}
				continue
			}
			f.pos = append(f.pos, w)
		}
		f.set = len(f.pos) > 0 || len(f.neg) > 0
	}
	return f
}

// active reports whether the filter hides any line.
func (f logFilter) active() bool {
	if !f.set {
		return false
	}
	return !(f.mode == filterRegex && f.invalid)
}

// match reports whether the plain (colour-free) line stays visible.
func (f logFilter) match(plain string) bool {
	if !f.active() {
		return true
	}
	switch f.mode {
	case filterRegex:
		return f.re.MatchString(plain)
	case filterFuzzy:
		sc, ok := fuzzyScore(f.fuzzy, plain)
		hit := ok && sc >= fuzzyMinPerRune*utf8.RuneCountInString(f.fuzzy)
		return hit != f.fuzzyNg
	}
	low := strings.ToLower(plain)
	for _, w := range f.pos {
		if !strings.Contains(low, w) {
			return false
		}
	}
	for _, w := range f.neg {
		if strings.Contains(low, w) {
			return false
		}
	}
	return true
}

// spans returns the highlight spans for the filter's positive terms.
func (f logFilter) spans(plain string) []logSpan {
	if !f.active() {
		return nil
	}
	switch f.mode {
	case filterRegex:
		var out []logSpan
		for _, m := range f.re.FindAllStringIndex(plain, -1) {
			if m[1] > m[0] {
				out = append(out, logSpan{
					utf8.RuneCountInString(plain[:m[0]]), utf8.RuneCountInString(plain[:m[1]]), spanFind,
				})
			}
		}
		return out
	case filterFuzzy:
		if f.fuzzyNg {
			return nil
		}
		return findSpans(plain, f.fuzzy)
	}
	var out []logSpan
	for _, w := range f.pos {
		out = append(out, findSpans(plain, w)...)
	}
	return out
}

// logFilterState is the filter of one log pane plus the index of the lines it
// keeps. vis lists the kept line indexes; visN is how many lines of the buffer
// it has looked at, so new lines are tested as they arrive.
type logFilterState struct {
	raw  string // text in the prompt / applied
	prev string // text to restore when the prompt is cancelled
	f    logFilter
	vis  []int
	visN int
}

// sync tests lines it has not seen yet. A buffer that shrank is rescanned.
func (s logFilterState) sync(lines []string) logFilterState {
	if !s.f.active() {
		s.vis, s.visN = nil, 0
		return s
	}
	if len(lines) < s.visN {
		s.vis, s.visN = nil, 0
	}
	for i := s.visN; i < len(lines); i++ {
		if s.f.match(plainLogLine(lines[i])) {
			s.vis = append(s.vis, i)
		}
	}
	s.visN = len(lines)
	return s
}

// withText replaces the filter text and rescans lines.
func (s logFilterState) withText(raw string, lines []string) logFilterState {
	s.raw = raw
	s.f = parseLogFilter(raw)
	s.vis, s.visN = nil, 0
	return s.sync(lines)
}
