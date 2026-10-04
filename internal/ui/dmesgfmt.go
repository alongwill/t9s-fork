package ui

import (
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// reDmesg matches the line Talos's Dmesg service formats for every kernel
// message: "<facility>: <priority, right-aligned to 7>: [<RFC 3339 time>]: <text>"
// (machined's v1alpha1 server, `%s: %7s: [%s]: %s`). The CLI may put "<node>: "
// in front. Groups: facility, level, timestamp.
var reDmesg = regexp.MustCompile(`^(?:\S+: )?([a-z][a-z0-9]*):\s+(emerg|alert|crit|err|warning|notice|info|debug): \[([^\]]*)\]: `)

// dmesgLine is the parsed head of a dmesg line. Offsets are byte offsets.
type dmesgLine struct {
	facility   [2]int
	level      [2]int
	stamp      [2]int
	levelKind  logSpanKind
	levelName  string
	facilityTx string
	ts         time.Time
	tsOK       bool
}

// parseDmesgLine reads facility, level and kernel timestamp from a line.
func parseDmesgLine(line string) (dmesgLine, bool) {
	m := reDmesg.FindStringSubmatchIndex(line)
	if m == nil {
		return dmesgLine{}, false
	}
	d := dmesgLine{
		facility:   [2]int{m[2], m[3]},
		level:      [2]int{m[4], m[5]},
		stamp:      [2]int{m[6], m[7]},
		levelName:  line[m[4]:m[5]],
		facilityTx: line[m[2]:m[3]],
	}
	d.levelKind = dmesgLevelKind(d.levelName)
	if t, err := time.Parse(time.RFC3339Nano, line[m[6]:m[7]]); err == nil {
		d.ts, d.tsOK = t.UTC(), true
	}
	return d, true
}

// dmesgLevelKind maps a kernel priority name to a colour: emerg/alert/crit/err
// red, warning yellow, notice/info blue, debug dim.
func dmesgLevelKind(level string) logSpanKind {
	switch level {
	case "emerg", "alert", "crit", "err":
		return spanErr
	case "warning":
		return spanWarn
	case "debug":
		return spanDebug
	}
	return spanInfo
}

// dmesgSpans colours the level token, and dims the facility and the kernel
// timestamp. A debug line is dimmed as a whole. Lines that do not have the
// dmesg shape (an error from the stream, say) fall back to the service log
// colouring. Offsets are rune offsets.
func dmesgSpans(line string) (spans []logSpan, dim bool) {
	d, ok := parseDmesgLine(line)
	if !ok {
		return logSpans(line)
	}
	rc := func(b int) int { return utf8.RuneCountInString(line[:b]) }
	spans = []logSpan{
		{rc(d.facility[0]), rc(d.facility[1]), spanTime},
		{rc(d.level[0]), rc(d.level[1]), d.levelKind},
		{rc(d.stamp[0] - 1), rc(d.stamp[1] + 1), spanTime}, // with the brackets
	}
	return spans, d.levelKind == spanDebug
}

// dmesgTimestamp is the Timestamps prefix: the kernel's own timestamp
// normalised to the log format, else the arrival time marked with '~'.
func dmesgTimestamp(line string, arrived time.Time) string {
	if d, ok := parseDmesgLine(line); ok && d.tsOK {
		return " " + d.ts.Format(logTSLayout)
	}
	if arrived.IsZero() {
		return strings.Repeat(" ", 1+len(logTSLayout))
	}
	return "~" + arrived.UTC().Format(logTSLayout)
}
