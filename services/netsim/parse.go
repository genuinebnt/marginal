package netsim

import (
	"strconv"
	"strings"
)

// ParseScenario reads § 14's editable script: one edit per line,
//
//	at, actor, insert|delete, pos, text-or-length
//
// Malformed lines are skipped and counted, never fatal — the same
// rule § 12's stream follows, and for the same reason: this is a
// text box somebody is typing into, and half a line is its normal
// state. A parser that refused the whole script mid-keystroke would
// break the screen exactly while it is being used.
func ParseScenario(src string) (edits []Edit, skipped int) {
	for _, line := range strings.Split(src, "\n") {
		// Leading space and a stray \r only. TrimSpace here would eat a
		// TRAILING space, which in an insert is a character somebody
		// typed — "quite " and "quite" are different edits.
		line = strings.TrimRight(line, "\r")
		line = strings.TrimLeft(line, " \t")
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Split at most 5 ways: inserted text may contain commas, and
		// splitting it into pieces would make "a, b" untypeable.
		parts := strings.SplitN(line, ",", 5)
		if len(parts) < 4 {
			skipped++
			continue
		}
		at, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		actor := strings.TrimSpace(parts[1])
		kind := Kind(strings.ToLower(strings.TrimSpace(parts[2])))
		pos, err2 := strconv.Atoi(strings.TrimSpace(parts[3]))
		if err1 != nil || err2 != nil || actor == "" ||
			(kind != Insert && kind != Delete) {
			skipped++
			continue
		}
		e := Edit{At: at, Actor: actor, Kind: kind, Pos: pos}
		last := ""
		if len(parts) > 4 {
			last = parts[4]
		}
		if kind == Insert {
			// Only the leading space is eaten — a trailing space in
			// inserted text is a character somebody typed, and
			// trimming it would silently change the document.
			e.Text = strings.TrimPrefix(last, " ")
			if e.Text == "" {
				skipped++
				continue
			}
		} else {
			n, err := strconv.Atoi(strings.TrimSpace(last))
			if err != nil || n <= 0 {
				skipped++
				continue
			}
			e.Len = n
		}
		edits = append(edits, e)
	}
	return edits, skipped
}
