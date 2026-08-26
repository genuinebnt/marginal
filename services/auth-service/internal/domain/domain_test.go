package domain

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEmailLowercasesAndTrims(t *testing.T) {
	e, err := NewEmail("  Alice@Example.COM  ")
	require.NoError(t, err)
	assert.Equal(t, "alice@example.com", e.String())
}

func TestNewEmailRejectsMalformed(t *testing.T) {
	_, err := NewEmail("not-an-email")
	assert.Error(t, err)
}

func TestNewEmailRejectsDisplayNameForm(t *testing.T) {
	// mail.ParseAddress happily accepts "Name <addr>" — a login identifier
	// must be a bare address, not a message header.
	_, err := NewEmail("Alice <alice@example.com>")
	assert.Error(t, err)
}

func TestNewPasswordRejectsTooShort(t *testing.T) {
	_, err := NewPassword("short")
	assert.Error(t, err)
}

func TestNewPasswordAcceptsLongPassphrase(t *testing.T) {
	// OWASP: a low maximum is a bug because it blocks passphrases.
	long := strings.Repeat("correct horse battery staple ", 3)
	_, err := NewPassword(long)
	assert.NoError(t, err)
}

func TestNewPasswordRejectsOverMax(t *testing.T) {
	_, err := NewPassword(strings.Repeat("a", maxPasswordLength+1))
	assert.Error(t, err)
}

func TestPasswordIsNeverPrintedInTheClear(t *testing.T) {
	p, err := NewPassword("hunter22222")
	require.NoError(t, err)

	assert.Equal(t, "[REDACTED]", fmt.Sprintf("%v", p))
	assert.Equal(t, "[REDACTED]", p.String())
	assert.Equal(t, "[REDACTED]", fmt.Sprintf("%#v", p))
	assert.NotContains(t, fmt.Sprintf("%v", p), "hunter2")

	assert.Equal(t, "hunter22222", p.Expose())
}

func TestNewDisplayNameRejectsEmpty(t *testing.T) {
	_, err := NewDisplayName("   ")
	assert.Error(t, err)
}

func TestNewDisplayNameRejectsControlCharacters(t *testing.T) {
	_, err := NewDisplayName("Alice\x00")
	assert.Error(t, err)
}

func TestAssignCursorColorIsDeterministic(t *testing.T) {
	id := NewUserID()
	first := AssignCursorColor(id)
	second := AssignCursorColor(id)
	assert.Equal(t, first, second, "the same user must always get the same color")
}

func TestAssignCursorColorIsAlwaysFromThePalette(t *testing.T) {
	for i := 0; i < 50; i++ {
		color := AssignCursorColor(NewUserID())
		assert.Contains(t, CursorPalette(), color)
	}
}
