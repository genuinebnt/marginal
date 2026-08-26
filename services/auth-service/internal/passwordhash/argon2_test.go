package passwordhash

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"marginal/auth-service/internal/domain"
)

// testParams keeps unit tests fast — real Argon2id cost is exercised by
// the timing test below, which needs DefaultParams to be meaningful.
var testParams = Params{Memory: 8 * 1024, Time: 1, Threads: 1, KeyLen: 32, SaltLen: 16}

func mustPassword(t *testing.T, raw string) domain.Password {
	t.Helper()
	p, err := domain.NewPassword(raw)
	require.NoError(t, err)
	return p
}

func TestHashVerifyRoundTrip(t *testing.T) {
	pw := mustPassword(t, "correct horse battery staple")
	hash, err := Hash(pw, testParams)
	require.NoError(t, err)

	ok, err := Verify(hash, pw)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestVerifyRejectsWrongPassword(t *testing.T) {
	hash, err := Hash(mustPassword(t, "correct horse battery staple"), testParams)
	require.NoError(t, err)

	ok, err := Verify(hash, mustPassword(t, "wrong password entirely"))
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestHashProducesDifferentSaltsEachTime(t *testing.T) {
	pw := mustPassword(t, "correct horse battery staple")
	a, err := Hash(pw, testParams)
	require.NoError(t, err)
	b, err := Hash(pw, testParams)
	require.NoError(t, err)
	assert.NotEqual(t, a.String(), b.String(), "two hashes of the same password must not collide on salt")
}

func TestVerifyRejectsMalformedPHCString(t *testing.T) {
	_, err := Verify(domain.NewPasswordHash("not-a-phc-string"), mustPassword(t, "irrelevant password"))
	assert.Error(t, err)
}

func TestDummyNeverMatches(t *testing.T) {
	dummy, err := NewDummy(testParams)
	require.NoError(t, err)
	// Verify has no observable return here by design — it exists purely
	// for its timing cost. Just confirm it doesn't panic or error.
	dummy.Verify(mustPassword(t, "anything at all"))
}

// TestUnknownEmailAndWrongPasswordTakeSimilarTime is the test named in
// docs/architecture/lld/auth-service.md §10 that "must fail if the
// dummy-hash verification is removed." Statistical, generous tolerance —
// skipped in -short mode since real Argon2id cost makes it slow.
func TestUnknownEmailAndWrongPasswordTakeSimilarTime(t *testing.T) {
	if testing.Short() {
		t.Skip("timing sample needs real Argon2id cost; skipped in -short")
	}

	real, err := Hash(mustPassword(t, "the real password"), DefaultParams())
	require.NoError(t, err)
	dummy, err := NewDummy(DefaultParams())
	require.NoError(t, err)

	const samples = 8
	wrongPasswordTotal := time.Duration(0)
	unknownEmailTotal := time.Duration(0)

	for i := 0; i < samples; i++ {
		start := time.Now()
		_, _ = Verify(real, mustPassword(t, "definitely the wrong password"))
		wrongPasswordTotal += time.Since(start)

		start = time.Now()
		dummy.Verify(mustPassword(t, "definitely the wrong password"))
		unknownEmailTotal += time.Since(start)
	}

	wrongAvg := wrongPasswordTotal / samples
	unknownAvg := unknownEmailTotal / samples
	ratio := float64(unknownAvg) / float64(wrongAvg)

	t.Logf("wrong-password avg=%s unknown-email(dummy) avg=%s ratio=%.2f", wrongAvg, unknownAvg, ratio)
	assert.InDelta(t, 1.0, ratio, 0.5, "dummy-hash path must cost about the same as a real verify, or it leaks which case occurred")
}
