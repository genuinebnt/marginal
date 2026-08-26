// Package authrest is auth.md §2's gateway REST mapping — a thin
// translation to/from auth-service's AuthService gRPC contract (§1 of
// that doc), which stays the source of truth for semantics.
package authrest

import (
	"time"

	authv1 "marginal/auth-service/genproto/authv1"

	"google.golang.org/protobuf/types/known/timestamppb"
)

type tokenPairJSON struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

func toTokenPairJSON(t *authv1.TokenPair) tokenPairJSON {
	return tokenPairJSON{
		AccessToken:  t.GetAccessToken(),
		RefreshToken: t.GetRefreshToken(),
		ExpiresIn:    t.GetExpiresIn(),
	}
}

// userJSON deliberately has no password field of any kind — auth.md §2:
// "password_hash ... never leaves auth-service at all, on any transport."
type userJSON struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	CursorColor string `json:"cursor_color"`
	CreatedAt   string `json:"created_at"`
}

func toUserJSON(u *authv1.User) userJSON {
	return userJSON{
		ID:          u.GetId(),
		Email:       u.GetEmail(),
		DisplayName: u.GetDisplayName(),
		CursorColor: u.GetCursorColor(),
		CreatedAt:   formatTimestamp(u.GetCreatedAt()),
	}
}

func formatTimestamp(ts *timestamppb.Timestamp) string {
	if ts == nil {
		return ""
	}
	return ts.AsTime().UTC().Format(time.RFC3339Nano)
}
