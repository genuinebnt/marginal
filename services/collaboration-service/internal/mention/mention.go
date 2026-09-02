// Package mention finds @handles in a comment body.
//
// It is deliberately a pure function over a string, with no database and no
// notion of who exists: resolving a handle to a person needs the page's
// space membership (comments.go does that), and mixing the two would make
// the grammar untestable without a running auth-service.
//
// The grammar is narrow on purpose. § 20's inbox is built on the promise
// that "anything routinely expiring unread should not have been a
// notification", and the cheapest way to break that promise is a parser
// that fires on text nobody meant as a mention.
package mention

import "strings"

// handleRune is the set a handle may contain. Deliberately no space: a
// display name of "Ada Lovelace" is mentioned as @adalovelace, because a
// space-bearing handle cannot be told from the sentence it sits in without
// knowing every name in advance — and the parser is not allowed to know.
func handleRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case r == '.' || r == '_' || r == '-':
		return true
	}
	return false
}

// Normalize is the one place a display name and a typed handle are made
// comparable: lowercase, and every space removed. Both sides go through
// it, so "@AdaLovelace" finds "Ada Lovelace" and nothing else has to know
// how the two differ.
func Normalize(s string) string {
	return strings.ToLower(strings.ReplaceAll(s, " ", ""))
}

// Parse returns the normalized handles mentioned in body, in the order
// they first appear, without duplicates.
//
// An @ that follows a word character is NOT a mention — that is an email
// address, and notifying somebody because their comment contained
// "ada@example.com" is exactly the noise the inbox is meant not to have.
func Parse(body string) []string {
	var out []string
	seen := map[string]bool{}
	runes := []rune(body)
	for i := 0; i < len(runes); i++ {
		if runes[i] != '@' {
			continue
		}
		// An @ glued to the end of a word is an address, not a mention.
		if i > 0 && handleRune(runes[i-1]) {
			continue
		}
		j := i + 1
		for j < len(runes) && handleRune(runes[j]) {
			j++
		}
		// Trailing punctuation is part of the sentence, not the name:
		// "@ada." and "@ada" are the same mention. Leading is impossible
		// (the run starts right after the @), so only the tail is trimmed.
		h := Normalize(strings.TrimRight(string(runes[i+1:j]), ".-_"))
		i = j - 1
		if h == "" || seen[h] {
			continue
		}
		seen[h] = true
		out = append(out, h)
	}
	return out
}
