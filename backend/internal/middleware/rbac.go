package middleware

import (
	"context"
	"net/http"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
)

// RequireCommander middleware ensures user is a commander (3SG+) or superadmin
func RequireCommander(db *database.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := GetUserFromContext(r.Context())
			if !ok {
				http.Error(w, "Not authenticated", http.StatusUnauthorized)
				return
			}

			if !user.IsCommander() && !user.IsSuperadmin {
				http.Error(w, "Insufficient permissions: Commander access required", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireSuperadmin middleware ensures user is a superadmin
func RequireSuperadmin(db *database.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := GetUserFromContext(r.Context())
			if !ok {
				http.Error(w, "Not authenticated", http.StatusUnauthorized)
				return
			}

			if !user.IsSuperadmin {
				http.Error(w, "Insufficient permissions: Superadmin access required", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// GetUserFromContext retrieves the user from request context
func GetUserFromContext(ctx context.Context) (*models.User, bool) {
	user, ok := ctx.Value(UserKey).(*models.User)
	return user, ok
}

// LoadUser middleware loads the full user object into context
// This should be used after Auth middleware
func LoadUser(db *database.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := r.Context().Value(UserIDKey).(string)
			if !ok {
				http.Error(w, "Not authenticated", http.StatusUnauthorized)
				return
			}

			ctx := r.Context()

			// Load full user object
			var user models.User
			var fullName, rank, battery *string
			var nricLast5, dob *string

			err := db.Pool.QueryRow(ctx, `
				SELECT
					id, "full_name",
					rank, battery, "nric_last5", dob, "is_superadmin",
					"createdAt", "updatedAt"
				FROM "user"
				WHERE id = $1
			`, userID).Scan(
				&user.ID, &fullName,
				&rank, &battery, &nricLast5, &dob, &user.IsSuperadmin,
				&user.CreatedAt, &user.UpdatedAt,
			)

			if err != nil {
				http.Error(w, "Failed to load user", http.StatusInternalServerError)
				return
			}

			user.FullName = fullName
			user.Rank = rank
			user.Battery = battery
			user.NRICLast5 = nricLast5
			user.DOB = dob

			// Add user to context
			ctx = context.WithValue(ctx, UserKey, &user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
