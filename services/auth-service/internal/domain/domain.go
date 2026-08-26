// Package domain holds auth-service's validated newtypes — zero I/O,
// unit-testable, matching docs/architecture/lld/auth-service.md §3
// (originally written for Rust; the invariants carry over unchanged, only
// the shape is Go).
package domain

import (
	"fmt"
	"hash/fnv"
	"net/mail"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
)

// Email is syntactically valid, lowercased and trimmed. Uniqueness is the
// database's job, not this type's.
type Email struct{ value string }

type InvalidEmailError struct{ Reason string }

func (e *InvalidEmailError) Error() string { return "invalid email: " + e.Reason }

func NewEmail(raw string) (Email, error) {
	trimmed := strings.TrimSpace(raw)
	addr, err := mail.ParseAddress(trimmed)
	if err != nil {
		return Email{}, &InvalidEmailError{Reason: "not a syntactically valid address"}
	}
	if addr.Address != trimmed {
		// mail.ParseAddress accepts "Display Name <addr>" — reject
		// anything that isn't a bare address, this is a login identifier
		// not a message header.
		return Email{}, &InvalidEmailError{Reason: "must be a bare address, no display name"}
	}
	return Email{value: strings.ToLower(trimmed)}, nil
}

func (e Email) String() string { return e.value }

// EmailFromStored reconstructs an Email read back from storage — trusted,
// not re-validated, since it was only ever written by NewEmail.
func EmailFromStored(value string) Email { return Email{value: value} }

const (
	minPasswordLength = 8   // OWASP Password Storage Cheat Sheet minimum
	maxPasswordLength = 128 // bounds Argon2's input cost, never below OWASP's 64-char floor
)

// Password is a plaintext password, held only long enough to hash or
// verify it. Deliberately not fmt.Stringer-transparent: String/GoString
// return a redacted placeholder so an accidental %v/%+v on a struct that
// embeds this (a log line, a panic message) can't leak it. There is no
// composition-rule validation — OWASP recommends against them; length is
// the only enforced policy.
type Password struct{ value string }

type InvalidPasswordError struct{ Reason string }

func (e *InvalidPasswordError) Error() string { return "invalid password: " + e.Reason }

func NewPassword(raw string) (Password, error) {
	n := utf8.RuneCountInString(raw)
	if n < minPasswordLength {
		return Password{}, &InvalidPasswordError{Reason: fmt.Sprintf("must be at least %d characters", minPasswordLength)}
	}
	if n > maxPasswordLength {
		return Password{}, &InvalidPasswordError{Reason: fmt.Sprintf("must be at most %d characters", maxPasswordLength)}
	}
	return Password{value: raw}, nil
}

// Expose returns the plaintext — call this only at the point of hashing or
// verification, never to log, store, or pass further than necessary.
func (p Password) Expose() string { return p.value }

func (p Password) String() string   { return "[REDACTED]" }
func (p Password) GoString() string { return "[REDACTED]" }

// PasswordHash is a PHC-format string: $argon2id$v=19$m=…,t=…,p=…$salt$hash.
// Carries algorithm, parameters, and salt together, so parameters can be
// upgraded without a schema migration (DATA_MODEL.md §3). Opaque to
// callers — internal/argon2hash is the only package that produces or
// verifies one.
type PasswordHash struct{ value string }

func NewPasswordHash(phc string) PasswordHash { return PasswordHash{value: phc} }
func (h PasswordHash) String() string         { return h.value }

// UserID is a user's identity — UUIDv7, generated in Go, never by a
// database default (same reasoning as pages.PageID: the id is needed
// before some rows can be constructed).
type UserID uuid.UUID

func NewUserID() UserID          { return UserID(uuid.Must(uuid.NewV7())) }
func (id UserID) String() string { return uuid.UUID(id).String() }

func ParseUserID(s string) (UserID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return UserID{}, fmt.Errorf("domain: invalid user id %q: %w", s, err)
	}
	return UserID(id), nil
}

// Jti is the JWT ID claim. The blocklist key is derived from it, so it
// must be unguessable — generated fresh per token, never derived from the
// user id.
type Jti uuid.UUID

func NewJti() Jti            { return Jti(uuid.New()) }
func (j Jti) String() string { return uuid.UUID(j).String() }

func ParseJti(s string) (Jti, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return Jti{}, fmt.Errorf("domain: invalid jti %q: %w", s, err)
	}
	return Jti(id), nil
}

const (
	minDisplayNameLength = 1
	maxDisplayNameBytes  = 200
)

type DisplayName struct{ value string }

type InvalidDisplayNameError struct{ Reason string }

func (e *InvalidDisplayNameError) Error() string { return "invalid display name: " + e.Reason }

func NewDisplayName(raw string) (DisplayName, error) {
	trimmed := strings.TrimSpace(raw)
	if utf8.RuneCountInString(trimmed) < minDisplayNameLength {
		return DisplayName{}, &InvalidDisplayNameError{Reason: "must not be empty"}
	}
	if len(trimmed) > maxDisplayNameBytes {
		return DisplayName{}, &InvalidDisplayNameError{Reason: fmt.Sprintf("must not exceed %d bytes", maxDisplayNameBytes)}
	}
	for _, r := range trimmed {
		if unicode.IsControl(r) {
			return DisplayName{}, &InvalidDisplayNameError{Reason: "must not contain control characters"}
		}
	}
	return DisplayName{value: trimmed}, nil
}

func (d DisplayName) String() string { return d.value }

// DisplayNameFromStored reconstructs a DisplayName read back from
// storage — trusted, not re-validated, since it was only ever written by
// NewDisplayName.
func DisplayNameFromStored(value string) DisplayName { return DisplayName{value: value} }

// CursorColor is one of CursorPalette's fixed hues — never an arbitrary
// color a caller supplies.
type CursorColor struct{ value string }

func (c CursorColor) String() string { return c.value }

// CursorColorFromStored reconstructs a CursorColor read back from
// storage — trusted, not re-validated against the palette, since it was
// only ever written by AssignCursorColor in the first place.
func CursorColorFromStored(value string) CursorColor { return CursorColor{value: value} }

// CursorPalette is the fixed set every collaborator's cursor is assigned
// from — the convention collaborative editors (Google Docs, Figma, Notion)
// use so two active cursors are never confusingly similar. See
// docs/api/auth.md § Product-facing behavior.
var CursorPalette = []CursorColor{
	{"#E03131"}, // red
	{"#F08C00"}, // orange
	{"#F5C518"}, // yellow
	{"#2F9E44"}, // green
	{"#0C8599"}, // teal
	{"#1971C2"}, // blue
	{"#7048E8"}, // violet
	{"#D6336C"}, // pink
}

// AssignCursorColor picks a color deterministically from id, so it's
// stable across sessions without a separate stored assignment beyond the
// column DATA_MODEL.md §3 already has.
func AssignCursorColor(id UserID) CursorColor {
	h := fnv.New32a()
	raw := uuid.UUID(id)
	_, _ = h.Write(raw[:])
	return CursorPalette[h.Sum32()%uint32(len(CursorPalette))]
}
