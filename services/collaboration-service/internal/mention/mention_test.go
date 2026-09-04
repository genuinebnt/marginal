package mention

import (
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       []string
	}{
		{"plain", "@ada does this hold?", []string{"ada"}},
		{"mid sentence", "ask @reiko about it", []string{"reiko"}},
		{"case folds", "@AdaLovelace", []string{"adalovelace"}},
		{"trailing punctuation is the sentence", "thanks @ada.", []string{"ada"}},
		{"two people", "@ada @reiko", []string{"ada", "reiko"}},
		{"the same person twice is one mention", "@ada and @ada again", []string{"ada"}},
		{"dots and dashes inside are kept", "@ada-l.ovelace", []string{"ada-l.ovelace"}},

		// The whole reason the grammar is narrow. Each of these looked like
		// a mention to an earlier, sloppier rule.
		{"an email address is not a mention", "write to ada@example.com", nil},
		{"a bare @ is not a mention", "cost @ 40%", nil},
		{"@ at the very end", "who owns this @", nil},
		{"no mentions at all", "the tiebreak still holds", nil},
		{"an @ inside a word", "user@host and me@here", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.body)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("Parse(%q) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}

// The hardest one: a display name and the handle somebody types for it are
// different strings, and exactly one function is allowed to know that.
func TestNormalizeMatchesDisplayNames(t *testing.T) {
	for _, tc := range []struct{ display, typed string }{
		{"Ada Lovelace", "@AdaLovelace"},
		{"ada", "@ADA"},
		{"Reiko  Tanaka", "@ReikoTanaka"},
	} {
		typed := Parse(tc.typed)
		if len(typed) != 1 {
			t.Fatalf("Parse(%q) produced %v", tc.typed, typed)
		}
		if got := Normalize(tc.display); got != typed[0] {
			t.Errorf("display %q normalises to %q, but %q parses to %q",
				tc.display, got, tc.typed, typed[0])
		}
	}
}

func FuzzParseNeverPanics(f *testing.F) {
	for _, s := range []string{"@ada", "a@b.com", "@@@", "@ада", "@", ""} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, body string) {
		for _, h := range Parse(body) {
			if h == "" {
				t.Fatal("Parse returned an empty handle")
			}
			if h != Normalize(h) {
				t.Fatalf("Parse returned %q, which is not normalised", h)
			}
		}
	})
}
