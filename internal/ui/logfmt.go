package ui

import (
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// logTSLayout is the prefix shown by the Timestamps toggle.
const logTSLayout = "2006-01-02T15:04:05.000"

// logTSPrefixW is the display width of the timestamp column: a marker cell
// ('~' for an arrival time, ' ' for a time parsed from the line), the 23
// character timestamp and one space.
const logTSPrefixW = 1 + len(logTSLayout) + 1

var (
	reTSJSON  = regexp.MustCompile(`"(?:ts|time|timestamp)"\s*:\s*("[^"]+"|\d+(?:\.\d+)?)`)
	reTSRFC   = regexp.MustCompile(`\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?(?:Z|[+-]\d{2}:?\d{2})?`)
	reTSGo    = regexp.MustCompile(`\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)?`)
	reTSKlog  = regexp.MustCompile(`^[EWIDF](\d{2})(\d{2}) (\d{2}):(\d{2}):(\d{2})(?:\.(\d+))?`)
	reLvlKV   = regexp.MustCompile(`(?i)["']?(?:level|lvl|severity)["']?\s*[=:]\s*["']?(error|err|fatal|crit|critical|warn|warning|info|debug|trace)\b`)
	reLvlBrk  = regexp.MustCompile(`\[(?:ERROR|FATAL|CRIT|CRITICAL|WARN|WARNING|INFO|DEBUG|TRACE|error|fatal|crit|warn|warning|info|debug|trace)\]`)
	reLvlKlog = regexp.MustCompile(`^[EWIDF]\d{4}\b`)
	reLvlWord = regexp.MustCompile(`\b(?:ERROR|FATAL|CRIT|CRITICAL|WARN|WARNING|INFO|DEBUG|TRACE|error|fatal|crit|warn|warning|info|debug|trace)\b`)
	reKey     = regexp.MustCompile(`(?:^|[\s{,])([A-Za-z_][A-Za-z0-9_.\-]*)=`)
)

// parseLineTimestamp looks for a timestamp inside a log line, in the formats
// Talos services emit: JSON "ts"/"time"/"timestamp" (RFC 3339 or epoch
// seconds), RFC 3339, "2006/01/02 15:04:05" and klog "I1003 12:00:00.123456".
// ref supplies the year klog omits. The result is in UTC; [s, e) are the byte
// offsets of the matched text.
func parseLineTimestamp(line string, ref time.Time) (t time.Time, s, e int, ok bool) {
	if m := reTSJSON.FindStringSubmatchIndex(line); m != nil {
		raw := line[m[2]:m[3]]
		if strings.HasPrefix(raw, `"`) {
			if pt, pok := parseRFC(strings.Trim(raw, `"`)); pok {
				return pt, m[2], m[3], true
			}
		} else if f, err := strconv.ParseFloat(raw, 64); err == nil {
			// Epoch seconds, or milliseconds when too large to be seconds.
			if f > 1e11 {
				f /= 1000
			}
			sec, frac := math.Modf(f)
			return time.Unix(int64(sec), int64(frac*1e9)).UTC(), m[2], m[3], true
		}
	}
	if m := reTSRFC.FindStringIndex(line); m != nil {
		if pt, pok := parseRFC(line[m[0]:m[1]]); pok {
			return pt, m[0], m[1], true
		}
	}
	if m := reTSGo.FindStringIndex(line); m != nil {
		txt := line[m[0]:m[1]]
		layout := "2006/01/02 15:04:05"
		if strings.Contains(txt, ".") {
			layout = "2006/01/02 15:04:05.999999999"
		}
		if pt, err := time.ParseInLocation(layout, txt, time.UTC); err == nil {
			return pt, m[0], m[1], true
		}
	}
	if m := reTSKlog.FindStringSubmatch(line); m != nil {
		year := ref.UTC().Year()
		if ref.IsZero() {
			year = time.Now().UTC().Year()
		}
		mon, _ := strconv.Atoi(m[1])
		day, _ := strconv.Atoi(m[2])
		hh, _ := strconv.Atoi(m[3])
		mm, _ := strconv.Atoi(m[4])
		ss, _ := strconv.Atoi(m[5])
		ns := 0
		if m[6] != "" {
			frac := (m[6] + "000000000")[:9]
			ns, _ = strconv.Atoi(frac)
		}
		pt := time.Date(year, time.Month(mon), day, hh, mm, ss, ns, time.UTC)
		if pt.Month() == time.Month(mon) && pt.Day() == day {
			// The timestamp part starts after the 5 character level+date token.
			return pt, 6, len(m[0]), true
		}
	}
	return time.Time{}, 0, 0, false
}

func parseRFC(s string) (time.Time, bool) {
	s = strings.Replace(s, ",", ".", 1)
	if i := strings.IndexByte(s, ' '); i == 10 {
		s = s[:10] + "T" + s[11:]
	}
	for _, layout := range []string{
		"2006-01-02T15:04:05.999999999Z07:00",
		"2006-01-02T15:04:05.999999999Z0700",
		"2006-01-02T15:04:05.999999999",
	} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// logTimestamp returns the prefix text for a line: the time found in the line
// (marker ' ') or else the arrival time (marker '~').
func logTimestamp(line string, arrived time.Time) string {
	if t, _, _, ok := parseLineTimestamp(line, arrived); ok {
		return " " + t.UTC().Format(logTSLayout)
	}
	if arrived.IsZero() {
		return strings.Repeat(" ", 1+len(logTSLayout))
	}
	return "~" + arrived.UTC().Format(logTSLayout)
}

type logSpanKind int

const (
	spanErr logSpanKind = iota
	spanWarn
	spanInfo
	spanDebug
	spanTime
	spanKey
	spanFind
)

// logSpan marks the rune range [a, b) of a line.
type logSpan struct {
	a, b int
	kind logSpanKind
}

// logLevelKind maps a level word to its span kind. ok is false for unknown
// words.
func logLevelKind(word string) (logSpanKind, bool) {
	switch strings.ToLower(word) {
	case "error", "err", "fatal", "crit", "critical":
		return spanErr, true
	case "warn", "warning":
		return spanWarn, true
	case "info":
		return spanInfo, true
	case "debug", "trace":
		return spanDebug, true
	}
	return 0, false
}

// logSpans computes the colour spans of one log line: the level token, the
// embedded timestamp and logfmt key names. dim reports a DEBUG/TRACE line,
// which is dimmed as a whole. Offsets are rune offsets into line.
func logSpans(line string) (spans []logSpan, dim bool) {
	rc := func(byteOff int) int { return utf8.RuneCountInString(line[:byteOff]) }
	add := func(a, b int, k logSpanKind) {
		spans = append(spans, logSpan{rc(a), rc(b), k})
	}

	// Level: structured forms first, then brackets, klog, bare words.
	var lvl logSpanKind
	found := false
	if m := reLvlKV.FindStringSubmatchIndex(line); m != nil {
		if k, ok := logLevelKind(line[m[2]:m[3]]); ok {
			lvl, found = k, true
			add(m[2], m[3], k)
		}
	}
	if !found {
		if m := reLvlBrk.FindStringIndex(line); m != nil {
			if k, ok := logLevelKind(strings.Trim(line[m[0]:m[1]], "[]")); ok {
				lvl, found = k, true
				add(m[0], m[1], k)
			}
		}
	}
	if !found {
		if m := reLvlKlog.FindStringIndex(line); m != nil {
			switch line[0] {
			case 'E', 'F':
				lvl, found = spanErr, true
			case 'W':
				lvl, found = spanWarn, true
			case 'I':
				lvl, found = spanInfo, true
			case 'D':
				lvl, found = spanDebug, true
			}
			if found {
				add(m[0], m[1], lvl)
			}
		}
	}
	if !found {
		if m := reLvlWord.FindStringIndex(line); m != nil {
			if k, ok := logLevelKind(line[m[0]:m[1]]); ok {
				lvl, found = k, true
				add(m[0], m[1], k)
			}
		}
	}
	dim = found && lvl == spanDebug

	if _, s, e, ok := parseLineTimestamp(line, time.Time{}); ok {
		add(s, e, spanTime)
	}
	for _, m := range reKey.FindAllStringSubmatchIndex(line, -1) {
		add(m[2], m[3], spanKey)
	}
	return spans, dim
}

// findSpans returns the case-insensitive occurrences of q in line as rune
// spans.
func findSpans(line, q string) []logSpan {
	if q == "" {
		return nil
	}
	lr, qr := []rune(strings.ToLower(line)), []rune(strings.ToLower(q))
	if len(lr) != utf8.RuneCountInString(line) || len(qr) == 0 || len(lr) < len(qr) {
		return nil
	}
	var out []logSpan
	for i := 0; i+len(qr) <= len(lr); {
		match := true
		for j := range qr {
			if lr[i+j] != qr[j] {
				match = false
				break
			}
		}
		if match {
			out = append(out, logSpan{i, i + len(qr), spanFind})
			i += len(qr)
		} else {
			i++
		}
	}
	return out
}
