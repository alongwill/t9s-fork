package ui

import (
	"regexp"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// YAML and tree-text colouring. Both work on plain text: a mark per byte says
// which style applies, then runs of equal marks are rendered. Spans come from
// regexps, so they always fall on rune boundaries.

const (
	markPlain int8 = iota
	markHit        // search match: wins over everything
	markTopKey
	markKey
	markString
	markNumber
	markKeyword // true / false / null
	markComment
	markPunct // list dashes, block scalar indicators, document markers
	markTypeName
	markTypeSuffix
	markArrow
	markPkg
)

var (
	yamlTopKeyStyle  = lipgloss.NewStyle().Foreground(colorCyan).Bold(true)
	yamlStringStyle  = lipgloss.NewStyle().Foreground(colorGreen)
	yamlNumberStyle  = lipgloss.NewStyle().Foreground(colorOrange)
	yamlKeywordStyle = lipgloss.NewStyle().Foreground(colorMagenta)
	yamlCommentStyle = lipgloss.NewStyle().Foreground(colorGray).Italic(true)
	yamlPunctStyle   = lipgloss.NewStyle().Foreground(colorGray)

	typeNameStyle   = lipgloss.NewStyle().Foreground(colorYellow).Bold(true)
	typeSuffixStyle = lipgloss.NewStyle().Foreground(colorGray)
	arrowStyle      = lipgloss.NewStyle().Foreground(colorCyan)
	pkgStyle        = lipgloss.NewStyle().Foreground(colorGray)
)

// yamlLineRe splits a line into indent, list dashes, an optional key and the
// rest (the value, or a bare scalar). A key is a plain word or a quoted
// string followed by `:` and whitespace or end of line, so `http://x` and
// `fe80::1` are not keys.
var yamlLineRe = regexp.MustCompile(`^(\s*)((?:-\s+)*)(?:("[^"]*"|'[^']*'|[A-Za-z0-9_./$@][^\s:]*):(?:\s+|$))?(.*)$`)

var (
	yamlNumberRe  = regexp.MustCompile(`^[-+]?(?:\d+\.?\d*|\.\d+)(?:[eE][-+]?\d+)?$`)
	yamlKeywordRe = regexp.MustCompile(`^(?:true|false|null|~)$`)
	yamlBlockRe   = regexp.MustCompile(`^(?:[|>][-+0-9]*|&\S+|\*\S+|!\S+)$`)
)

func fillMarks(marks []int8, from, to int, m int8) {
	for i := from; i < to && i < len(marks); i++ {
		marks[i] = m
	}
}

// markYAML classifies every byte of one YAML line.
func markYAML(text string, marks []int8) {
	m := yamlLineRe.FindStringSubmatchIndex(text)
	if m == nil {
		return
	}
	// group spans: 1 indent, 2 dashes, 3 key, 4 rest
	if m[5] > m[4] {
		for i := m[4]; i < m[5]; i++ {
			if text[i] == '-' {
				marks[i] = markPunct
			}
		}
	}
	if m[6] >= 0 {
		key := markKey
		if m[3] == m[2] && m[5] == m[4] { // no indent, no dashes
			key = markTopKey
		}
		fillMarks(marks, m[6], m[7], key)
	}
	rs, re := m[8], len(text)
	for re > rs && text[re-1] == ' ' {
		re--
	}
	if rs >= re {
		return
	}
	rest := text[rs:re]
	switch {
	case rest == "---" || rest == "...":
		fillMarks(marks, rs, re, markPunct)
		return
	case strings.HasPrefix(rest, "#"):
		fillMarks(marks, rs, re, markComment)
		return
	}
	val := rest
	if c := inlineCommentAt(rest); c >= 0 {
		fillMarks(marks, rs+c, re, markComment)
		val = strings.TrimRight(rest[:c], " ")
	}
	mk := markString
	switch {
	case yamlKeywordRe.MatchString(val):
		mk = markKeyword
	case yamlNumberRe.MatchString(val):
		mk = markNumber
	case yamlBlockRe.MatchString(val):
		mk = markPunct
	}
	fillMarks(marks, rs, rs+len(val), mk)
}

// inlineCommentAt returns the offset of ` #` that starts a trailing comment,
// or -1. Quoted values are left alone.
func inlineCommentAt(rest string) int {
	if strings.HasPrefix(rest, `"`) || strings.HasPrefix(rest, "'") {
		return -1
	}
	return strings.Index(rest, " #")
}

func markStyle(m int8, hit lipgloss.Style) (lipgloss.Style, bool) {
	switch m {
	case markHit:
		return hit, true
	case markTopKey:
		return yamlTopKeyStyle, true
	case markKey:
		return yamlKeyStyle, true
	case markString:
		return yamlStringStyle, true
	case markNumber:
		return yamlNumberStyle, true
	case markKeyword:
		return yamlKeywordStyle, true
	case markComment:
		return yamlCommentStyle, true
	case markPunct:
		return yamlPunctStyle, true
	case markTypeName:
		return typeNameStyle, true
	case markTypeSuffix:
		return typeSuffixStyle, true
	case markArrow:
		return arrowStyle, true
	case markPkg:
		return pkgStyle, true
	}
	return lipgloss.Style{}, false
}

// paint renders text with one style per run of equal marks.
func paint(text string, marks []int8, hit lipgloss.Style) string {
	var sb strings.Builder
	for i := 0; i < len(text); {
		j := i
		for j < len(text) && marks[j] == marks[i] {
			j++
		}
		if st, ok := markStyle(marks[i], hit); ok {
			sb.WriteString(st.Render(text[i:j]))
		} else {
			sb.WriteString(text[i:j])
		}
		i = j
	}
	return sb.String()
}

// colorYAMLLine colours one YAML line and highlights search matches. text may
// be padded to the pane width.
func colorYAMLLine(text string, find *regexp.Regexp, hs lipgloss.Style) string {
	marks := make([]int8, len(text))
	markYAML(text, marks)
	if find != nil {
		for _, loc := range find.FindAllStringIndex(text, -1) {
			if loc[1] > loc[0] {
				fillMarks(marks, loc[0], loc[1], markHit)
			}
		}
	}
	return paint(text, marks, hs)
}

// colorYAMLText colours a whole YAML document, one line at a time, with an
// optional case-insensitive literal search highlight.
func colorYAMLText(content, query string) string {
	var find *regexp.Regexp
	if query != "" {
		find = regexp.MustCompile("(?i)" + regexp.QuoteMeta(query))
	}
	lines := strings.Split(content, "\n")
	for i, l := range lines {
		lines[i] = colorYAMLLine(strings.TrimRight(l, "\r"), find, hitStyle)
	}
	return strings.Join(lines, "\n")
}

// --- describe tree text ---

var (
	// A full resource type: the display name in front of the group suffix.
	typeNameRe = regexp.MustCompile(`([A-Za-z0-9]+)(\.[a-z0-9.]*talos\.dev)`)
	// A controller: lower-case package, then the name.
	controllerRe = regexp.MustCompile(`\b([a-z0-9]+\.)([A-Z][A-Za-z0-9]*Controller)\b`)
)

// colorTreeText highlights resource names and arrows in describe text: the
// name of a type stands out and its group suffix recedes; a controller's
// package recedes.
func colorTreeText(text string) string {
	marks := make([]int8, len(text))
	for _, m := range typeNameRe.FindAllStringSubmatchIndex(text, -1) {
		fillMarks(marks, m[2], m[3], markTypeName)
		fillMarks(marks, m[4], m[5], markTypeSuffix)
	}
	for _, m := range controllerRe.FindAllStringSubmatchIndex(text, -1) {
		fillMarks(marks, m[2], m[3], markPkg)
	}
	for i, r := range text {
		if r == '◀' || r == '▶' {
			fillMarks(marks, i, i+len(string(r)), markArrow)
		}
	}
	return paint(text, marks, lipgloss.Style{})
}
