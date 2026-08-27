package domain

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEmail(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "lowercases and trims", input: "  Alice@Example.COM  ", want: "alice@example.com"},
		{name: "rejects malformed", input: "not-an-email", wantErr: true},
		{
			// mail.ParseAddress happily accepts "Name <addr>" — a login
			// identifier must be a bare address, not a message header.
			name:    "rejects display name form",
			input:   "Alice <alice@example.com>",
			wantErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e, err := NewEmail(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, e.String())
		})
	}
}

func TestNewPassword(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "rejects too short", input: "short", wantErr: true},
		{
			// OWASP: a low maximum is a bug because it blocks passphrases.
			name:  "accepts long passphrase",
			input: strings.Repeat("correct horse battery staple ", 3),
		},
		{name: "rejects over max", input: strings.Repeat("a", maxPasswordLength+1), wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewPassword(tc.input)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
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

func TestNewDisplayNameRejects(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "empty", input: "   "},
		{name: "control characters", input: "Alice\x00"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewDisplayName(tc.input)
			assert.Error(t, err)
		})
	}
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
