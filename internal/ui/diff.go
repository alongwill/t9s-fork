package ui

import (
	"fmt"
	"strings"
)

// diffLine is one line of a unified diff. op is ' ', '+', '-', '@' (hunk
// header) or 'h' (file header).
type diffLine struct {
	op   byte
	text string
}

// maxLCSCells bounds the LCS table. A differing region larger than this is
// reported as removed-then-added instead of matched line by line.
const maxLCSCells = 4_000_000

// editOp is one step of an edit script over two line lists.
type editOp struct {
	op   byte // ' ' keep, '-' delete from a, '+' insert from b
	text string
}

// lineDiff returns a shortest edit script turning a into b (small LCS diff:
// common prefix and suffix are trimmed first, so typical near-identical YAML
// documents only pay for the differing middle).
func lineDiff(a, b []string) []editOp {
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	am, bm := a[pre:len(a)-suf], b[pre:len(b)-suf]

	var out []editOp
	for _, l := range a[:pre] {
		out = append(out, editOp{' ', l})
	}
	out = append(out, middleDiff(am, bm)...)
	for _, l := range a[len(a)-suf:] {
		out = append(out, editOp{' ', l})
	}
	return out
}

func middleDiff(a, b []string) []editOp {
	n, m := len(a), len(b)
	switch {
	case n == 0 && m == 0:
		return nil
	case n == 0 || m == 0 || n*m > maxLCSCells:
		out := make([]editOp, 0, n+m)
		for _, l := range a {
			out = append(out, editOp{'-', l})
		}
		for _, l := range b {
			out = append(out, editOp{'+', l})
		}
		return out
	}
	// lcs[i][j] = LCS length of a[i:] and b[j:]
	lcs := make([][]int32, n+1)
	for i := range lcs {
		lcs[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	var out []editOp
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			out = append(out, editOp{' ', a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, editOp{'-', a[i]})
			i++
		default:
			out = append(out, editOp{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, editOp{'-', a[i]})
	}
	for ; j < m; j++ {
		out = append(out, editOp{'+', b[j]})
	}
	return out
}

// unifiedDiff renders the difference between two texts as unified-diff lines
// with ctx lines of context. Identical texts yield only the two header lines.
func unifiedDiff(nameA, nameB, a, b string, ctx int) []diffLine {
	out := []diffLine{{'h', "--- " + nameA}, {'h', "+++ " + nameB}}
	ops := lineDiff(splitLines(a), splitLines(b))

	// positions (1-based) of each op in a and b
	type posOp struct {
		editOp
		ai, bi int
	}
	pos := make([]posOp, len(ops))
	ai, bi := 1, 1
	for i, o := range ops {
		pos[i] = posOp{o, ai, bi}
		if o.op != '+' {
			ai++
		}
		if o.op != '-' {
			bi++
		}
	}

	i := 0
	for i < len(pos) {
		for i < len(pos) && pos[i].op == ' ' {
			i++
		}
		if i >= len(pos) {
			break
		}
		start := max(0, i-ctx)
		// extend to the end of the hunk: stop after > 2*ctx unchanged lines
		end := i
		for last := i; last < len(pos); last++ {
			if pos[last].op != ' ' {
				end = last
				continue
			}
			if last-end > 2*ctx {
				break
			}
		}
		stop := min(len(pos), end+ctx+1)
		var aLen, bLen int
		for _, p := range pos[start:stop] {
			if p.op != '+' {
				aLen++
			}
			if p.op != '-' {
				bLen++
			}
		}
		aStart, bStart := pos[start].ai, pos[start].bi
		if aLen == 0 {
			aStart--
		}
		if bLen == 0 {
			bStart--
		}
		out = append(out, diffLine{'@', fmt.Sprintf("@@ -%d,%d +%d,%d @@", aStart, aLen, bStart, bLen)})
		for _, p := range pos[start:stop] {
			out = append(out, diffLine{p.op, p.text})
		}
		i = stop
	}
	return out
}

func splitLines(s string) []string {
	s = strings.TrimRight(s, "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// diffChanged reports whether the diff contains any +/- line.
func diffChanged(lines []diffLine) bool {
	for _, l := range lines {
		if l.op == '+' || l.op == '-' {
			return true
		}
	}
	return false
}
