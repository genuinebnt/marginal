package wsapi

import "testing"

// Op kinds are scope-prefixed on the wire — RFC-002's two
// tiers. A bare-name classifier silently called every delete
// "content", and nothing failed: the DESTRUCTIVE filter just
// returned an empty list, which reads as "nothing was deleted"
// rather than as a bug.
func TestDestructiveIsRecognisedThroughTheTierPrefix(t *testing.T) {
	cases := map[string]string{
		"block:DeleteBlock": "destructive",
		"text:DeleteText":   "destructive",
		"DeleteBlock":       "destructive",
		"block:InsertBlock": "content",
		"text:InsertText":   "content",
		// Moving is not losing: MoveBlock carries `from` as well
		// as `to` and inverts exactly.
		"block:MoveBlock": "content",
		// An unknown kind is content, on purpose: a new op should
		// appear in the log as ordinary rather than vanish from
		// it because nobody classified it.
		"block:SomethingNew": "content",
	}
	for kind, want := range cases {
		if got := classOf(kind); got != want {
			t.Errorf("classOf(%q) = %q, want %q", kind, got, want)
		}
	}
}
