// Package passwordhash is the only package that produces or verifies a
// domain.PasswordHash — Argon2id per RFC 9106, encoded as a PHC string so
// algorithm, parameters, and salt travel together (DATA_MODEL.md §3: a
// future parameter increase doesn't need a migration).
package passwordhash

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"

	"marginal/auth-service/internal/domain"
)

// Params are a latency budget, not a security dial (docs/architecture/lld/auth-service.md
// §12) — verification runs synchronously on every login. DefaultParams is
// docs/api/auth.md's OWASP second-tier choice: lighter than the 64 MiB/p=4
// first tier, sized for small, cost-bounded Cloud Run instances.
type Params struct {
	Memory  uint32 // KiB
	Time    uint32 // iterations
	Threads uint8
	KeyLen  uint32
	SaltLen uint32
}

var DefaultParams = Params{
	Memory:  19 * 1024, // 19 MiB
	Time:    2,
	Threads: 1,
	KeyLen:  32,
	SaltLen: 16,
}

// Hash computes a new PHC-format hash for password using params. Call
// password.Expose() only here — this is the one legitimate place a
// plaintext password is read.
func Hash(password domain.Password, params Params) (domain.PasswordHash, error) {
	salt := make([]byte, params.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return domain.PasswordHash{}, fmt.Errorf("passwordhash: generating salt: %w", err)
	}
	sum := argon2.IDKey([]byte(password.Expose()), salt, params.Time, params.Memory, params.Threads, params.KeyLen)

	phc := fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, params.Memory, params.Time, params.Threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(sum),
	)
	return domain.NewPasswordHash(phc), nil
}

// Verify reports whether password matches hash. The comparison is
// constant-time (crypto/subtle) — a == on the raw bytes would leak the
// hash's shared-prefix length through timing.
func Verify(hash domain.PasswordHash, password domain.Password) (bool, error) {
	params, salt, sum, err := parse(hash.String())
	if err != nil {
		return false, err
	}
	candidate := argon2.IDKey([]byte(password.Expose()), salt, params.Time, params.Memory, params.Threads, uint32(len(sum)))
	return subtle.ConstantTimeCompare(candidate, sum) == 1, nil
}

func parse(phc string) (Params, []byte, []byte, error) {
	// $argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>
	parts := strings.Split(phc, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return Params{}, nil, nil, fmt.Errorf("passwordhash: not a recognized argon2id PHC string")
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return Params{}, nil, nil, fmt.Errorf("passwordhash: malformed version segment: %w", err)
	}
	if version != argon2.Version {
		return Params{}, nil, nil, fmt.Errorf("passwordhash: unsupported argon2 version %d", version)
	}

	var params Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &params.Memory, &params.Time, &params.Threads); err != nil {
		return Params{}, nil, nil, fmt.Errorf("passwordhash: malformed params segment: %w", err)
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return Params{}, nil, nil, fmt.Errorf("passwordhash: malformed salt: %w", err)
	}
	sum, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return Params{}, nil, nil, fmt.Errorf("passwordhash: malformed hash: %w", err)
	}
	return params, salt, sum, nil
}

// Dummy is a real Argon2id hash of a fixed string, computed once so the
// "unknown email" path can verify against *something* with the same cost
// as a genuine check — never an early return, which is what would make
// the timing distinguishable from a real wrong-password rejection
// (docs/architecture/lld/auth-service.md §9/§12).
type Dummy struct {
	hash domain.PasswordHash
}

func NewDummy(params Params) (*Dummy, error) {
	fixed, err := domain.NewPassword("dummy-password-for-constant-time-comparison")
	if err != nil {
		return nil, err
	}
	hash, err := Hash(fixed, params)
	if err != nil {
		return nil, err
	}
	return &Dummy{hash: hash}, nil
}

// Verify always returns false (it's a fixed hash unrelated to the
// supplied password) but costs the same as a real Verify call.
func (d *Dummy) Verify(password domain.Password) {
	_, _ = Verify(d.hash, password)
}
