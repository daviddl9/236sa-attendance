package middleware

import (
	"context"
	"net/http"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
)

type contextKey string

const UserIDKey contextKey = "userID"
const UserKey contextKey = "user"

func Auth(db *database.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get session token from cookie
			cookie, err := r.Cookie("session")
			if err != nil {
				http.Error(w, "Not authenticated", http.StatusUnauthorized)
				return
			}

			ctx := r.Context()

			// Validate session and get user ID
			var userID string
			err = db.Pool.QueryRow(ctx, `
				SELECT "userId" FROM session
				WHERE token = $1 AND "expiresAt" > NOW()
			`, cookie.Value).Scan(&userID)

			if err != nil {
				http.Error(w, "Invalid or expired session", http.StatusUnauthorized)
				return
			}

			// Add user ID to request context
			ctx = context.WithValue(ctx, UserIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
