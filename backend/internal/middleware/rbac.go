package middleware

import (
	"context"
	"net/http"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
)

// RequireBatteryNCO allows Tier 2+ (3SG–1SG, SSG+, superadmin).
func RequireBatteryNCO(db *database.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := GetUserFromContext(r.Context())
			if !ok {
				http.Error(w, "Not authenticated", http.StatusUnauthorized)
				return
			}
			if user.GetTier() < models.TierBatteryNCO {
				http.Error(w, "Insufficient permissions", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireUnitCommander allows Tier 3+ (SSG+, superadmin).
func RequireUnitCommander(db *database.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := GetUserFromContext(r.Context())
			if !ok {
				http.Error(w, "Not authenticated", http.StatusUnauthorized)
				return
			}
			if user.GetTier() < models.TierUnitCommander {
				http.Error(w, "Insufficient permissions", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireCommander is kept as an alias for RequireBatteryNCO for the SSE handler.
func RequireCommander(db *database.DB) func(http.Handler) http.Handler {
	return RequireBatteryNCO(db)
}

// RequireSuperadmin middleware ensures user is a superadmin.
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

// GetUserFromContext retrieves the user from request context.
func GetUserFromContext(ctx context.Context) (*models.User, bool) {
	user, ok := ctx.Value(UserKey).(*models.User)
	return user, ok
}

// RequirePasswordChange blocks authenticated application requests while a
// temporary credential is active. The change-password endpoint is the sole
// application route allowed through this guard.
func RequirePasswordChange(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, ok := GetUserFromContext(r.Context())
		if !ok {
			http.Error(w, "Not authenticated", http.StatusUnauthorized)
			return
		}
		if user.PasswordChangeRequired && !(r.Method == http.MethodPost && r.URL.Path == "/api/auth/change-password") {
			http.Error(w, "Password change required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// LoadUser middleware loads the full user object into context.
func LoadUser(db *database.DB) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			userID, ok := r.Context().Value(UserIDKey).(string)
			if !ok {
				http.Error(w, "Not authenticated", http.StatusUnauthorized)
				return
			}

			ctx := r.Context()

			var user models.User
			var fullName, rank, battery *string
			var nricLast5, dob *string
			var tierOverride *int16

			err := db.Pool.QueryRow(ctx, `
				SELECT
					id, "full_name",
					rank, battery, "nric_last5", dob, "is_superadmin",
					tier_override, verified, password_change_required,
					"createdAt", "updatedAt"
				FROM "user"
				WHERE id = $1
			`, userID).Scan(
				&user.ID, &fullName,
				&rank, &battery, &nricLast5, &dob, &user.IsSuperadmin,
				&tierOverride, &user.Verified, &user.PasswordChangeRequired,
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
			user.TierOverride = tierOverride

			ctx = context.WithValue(ctx, UserKey, &user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
