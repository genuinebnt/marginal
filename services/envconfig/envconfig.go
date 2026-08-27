// Package envconfig is the env-var-loading plumbing every service's
// cmd/main.go needs — EnvOr and RequiredEnv used to be duplicated
// byte-for-byte across all five (four, for the required-env check)
// cmd/main.go files. Unlike this repo's other deliberate per-service
// duplicates (collaboration-service/outbox's wireEvent,
// pageop/pageop.go's spliceStringField), each of which independently
// owns its own half of a wire contract that could legitimately diverge,
// this has no contract to diverge from — it's just os.Getenv, so one
// shared copy is correct, not premature abstraction.
package envconfig

import "os"

// EnvOr returns key's environment value, or fallback if it's unset or
// empty.
func EnvOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// RequiredEnvError reports that key was required but unset or empty.
type RequiredEnvError string

func (e RequiredEnvError) Error() string { return "missing required env var: " + string(e) }

// RequiredEnv returns key's environment value, or a RequiredEnvError if
// it's unset or empty.
func RequiredEnv(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", RequiredEnvError(key)
	}
	return v, nil
}
