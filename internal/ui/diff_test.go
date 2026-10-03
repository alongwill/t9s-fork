package ui

import (
	"strings"
	"testing"
)

func render(lines []diffLine) string {
	var sb strings.Builder
	for _, l := range lines {
		if l.op == 'h' || l.op == '@' {
			sb.WriteString(l.text)
		} else {
			sb.WriteByte(l.op)
			sb.WriteString(l.text)
		}
		sb.WriteByte('\n')
	}
	return sb.String()
}

func TestUnifiedDiffKnownPair(t *testing.T) {
	a := "spec:\n    address: 10.0.0.1/24\n    link: eth0\n    family: inet4\n    scope: global\n"
	b := "spec:\n    address: 10.0.0.2/24\n    link: eth0\n    family: inet4\n    scope: global\n    flags: permanent\n"
	want := "--- cp-1 (10.0.0.1)\n+++ cp-2 (10.0.0.2)\n" +
		"@@ -1,5 +1,6 @@\n" +
		" spec:\n" +
		"-    address: 10.0.0.1/24\n" +
		"+    address: 10.0.0.2/24\n" +
		"     link: eth0\n" +
		"     family: inet4\n" +
		"     scope: global\n" +
		"+    flags: permanent\n"
	got := render(unifiedDiff("cp-1 (10.0.0.1)", "cp-2 (10.0.0.2)", a, b, 3))
	if got != want {
		t.Fatalf("diff mismatch\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

func TestUnifiedDiffIdentical(t *testing.T) {
	lines := unifiedDiff("a", "b", "x: 1\ny: 2\n", "x: 1\ny: 2\n", 3)
	if diffChanged(lines) || len(lines) != 2 {
		t.Fatalf("identical texts must give headers only: %v", lines)
	}
}

func TestUnifiedDiffSplitsFarApartHunks(t *testing.T) {
	var a, b []string
	for i := 0; i < 30; i++ {
		a = append(a, "line")
		b = append(b, "line")
	}
	a[2], b[2] = "old-top", "new-top"
	a[27], b[27] = "old-bottom", "new-bottom"
	got := unifiedDiff("a", "b", strings.Join(a, "\n"), strings.Join(b, "\n"), 3)
	hunks := 0
	for _, l := range got {
		if l.op == '@' {
			hunks++
		}
	}
	if hunks != 2 {
		t.Fatalf("hunks = %d, want 2:\n%s", hunks, render(got))
	}
}

func TestLineDiffRoundTrips(t *testing.T) {
	a := strings.Split("a b c d e f g", " ")
	b := strings.Split("a c d x f g h", " ")
	var gotA, gotB []string
	for _, o := range lineDiff(a, b) {
		if o.op != '+' {
			gotA = append(gotA, o.text)
		}
		if o.op != '-' {
			gotB = append(gotB, o.text)
		}
	}
	if strings.Join(gotA, " ") != strings.Join(a, " ") || strings.Join(gotB, " ") != strings.Join(b, " ") {
		t.Fatalf("edit script does not reproduce the inputs: %v / %v", gotA, gotB)
	}
}

func TestLineDiffHugeMiddleFallsBack(t *testing.T) {
	var a, b []string
	for i := 0; i < 2500; i++ {
		a = append(a, "a"+strings.Repeat("x", i%7))
		b = append(b, "b"+strings.Repeat("y", i%5))
	}
	ops := lineDiff(a, b)
	if len(ops) != len(a)+len(b) {
		t.Fatalf("fallback should remove all then add all, got %d ops", len(ops))
	}
}

func TestUnifiedDiffAddToEmpty(t *testing.T) {
	got := render(unifiedDiff("a", "b", "", "x: 1\n", 3))
	if !strings.Contains(got, "@@ -0,0 +1,1 @@") || !strings.Contains(got, "+x: 1") {
		t.Fatalf("got:\n%s", got)
	}
}
