// Package users owns auth.users — registration and lookup. Password
// verification itself lives in internal/passwordhash; this package only
// stores and retrieves the PHC string.
package users

import (
	"time"

	"marginal/auth-service/internal/domain"
)

type User struct {
	ID           domain.UserID
	Email        domain.Email
	PasswordHash domain.PasswordHash
	DisplayName  domain.DisplayName
	CursorColor  domain.CursorColor
	CreatedAt    time.Time
}

type NewUser struct {
	ID           domain.UserID
	Email        domain.Email
	PasswordHash domain.PasswordHash
	DisplayName  domain.DisplayName
	CursorColor  domain.CursorColor
}
