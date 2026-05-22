package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/handlers"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/services/agent"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/sse"
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables from .env file (only if not already set)
	// godotenv.Load() does NOT override existing environment variables,
	// so Docker Compose env vars will take precedence
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Initialize database connection
	db, err := database.NewPostgresDB(os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	// Run database migrations
	migrationsDir := getEnv("MIGRATIONS_DIR", "./migrations")
	if err := database.RunMigrations(os.Getenv("DATABASE_URL"), migrationsDir); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	// Seed admin user
	if err := database.SeedAdminUser(db); err != nil {
		log.Printf("Warning: Failed to seed admin user: %v", err)
	}

	// Initialize SSE hub for real-time updates
	sseHub := sse.NewHub()

	// Initialize router
	r := chi.NewRouter()

	// Middleware
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	// Custom request/response logger
	r.Use(middleware.RequestResponseLogger)
	// Custom recoverer with logging
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					log.Printf("[PANIC] Recovered from panic: %v, Request: %s %s", err, r.Method, r.URL.Path)
					http.Error(w, "Internal server error", http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	})

	// CORS configuration for frontend (must be before timeout middleware)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{getEnv("FRONTEND_URL", "http://localhost:5173")},
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		ExposedHeaders:   []string{"Link", "Location"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Health check endpoint (no timeout needed)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// SSE stream endpoint - must be outside timeout middleware
	// because timeout middleware wraps ResponseWriter which breaks http.Flusher
	sseHandler := handlers.NewSSEHandler(db, sseHub)
	r.Route("/api/sessions/{id}/stream", func(r chi.Router) {
		r.Use(middleware.Auth(db))
		r.Use(middleware.LoadUser(db))
		r.Use(middleware.RequireCommander(db))
		r.Get("/", sseHandler.StreamSession)
	})

	// Agent-driven document import — Anthropic API calls can take 2-3 min for large rosters;
	// registered outside the 60s group with its own 5-minute timeout.
	importHandler := handlers.NewImportHandler(db, newAgentParser())
	r.Group(func(r chi.Router) {
		r.Use(chimiddleware.Timeout(5 * time.Minute))
		r.Route("/api/admin/users/import-document", func(r chi.Router) {
			r.Use(middleware.Auth(db))
			r.Use(middleware.LoadUser(db))
			r.Use(middleware.RequireSuperadmin(db))
			r.Post("/preview", importHandler.PreviewImport)
			r.Post("/commit", importHandler.CommitImport)
		})
	})

	// All other API routes with timeout middleware
	r.Group(func(r chi.Router) {
		r.Use(chimiddleware.Timeout(60 * time.Second))

		// API routes
		r.Route("/api", func(r chi.Router) {
			// Public QR scan route (must be before protected routes)
			attendanceHandler := handlers.NewAttendanceHandler(db, sseHub)
			r.Get("/qr/{token}", attendanceHandler.HandleQRScan)

			// Auth routes (public)
			r.Route("/auth", func(r chi.Router) {
				authHandler := handlers.NewAuthHandler(db)
				r.Post("/sign-in", authHandler.SignIn)
				r.Post("/sign-out", authHandler.SignOut)
				r.Get("/session", authHandler.GetSession)
				// Explicit signup — called after sign-in returns signup_required
				r.Post("/sign-up", authHandler.SignUp)
			})

			// User registration route (public)
			userHandler := handlers.NewUserHandler(db)
			r.Post("/users/register", userHandler.RegisterUser)

			// Protected routes
			r.Group(func(r chi.Router) {
				r.Use(middleware.Auth(db))
				r.Use(middleware.LoadUser(db))

				// User profile routes
				userHandler := handlers.NewUserHandler(db)
				r.Get("/user/profile", userHandler.GetProfile)
				r.Put("/user/profile", userHandler.UpdateProfile)

				// User management routes
				r.Route("/users", func(r chi.Router) {
					// View routes: Tier 2+ (battery-scoped automatically for Tier 2)
					r.With(middleware.RequireBatteryNCO(db)).Get("/", userHandler.ListUsers)
					r.With(middleware.RequireBatteryNCO(db)).Get("/bulk/count", userHandler.BulkDeleteCount)
					r.With(middleware.RequireBatteryNCO(db)).Get("/{id}", userHandler.GetUser)
					// Write routes: superadmin only
					r.With(middleware.RequireSuperadmin(db)).Put("/{id}", userHandler.UpdateUser)
					r.With(middleware.RequireSuperadmin(db)).Delete("/{id}", userHandler.DeleteUser)
					r.With(middleware.RequireSuperadmin(db)).Delete("/bulk", userHandler.BulkDelete)
				})

				// Attendance routes
				r.Post("/attendance/mark", attendanceHandler.MarkAttendance)

				// Session routes
				sessionHandler := handlers.NewSessionHandler(db, sseHub)
				r.Route("/sessions", func(r chi.Router) {
					// View routes: Tier 2+
					r.With(middleware.RequireBatteryNCO(db)).Get("/", sessionHandler.ListSessions)
					r.With(middleware.RequireBatteryNCO(db)).Get("/active", sessionHandler.GetActiveSessions)
					r.With(middleware.RequireBatteryNCO(db)).Get("/{id}", sessionHandler.GetSession)

					// Management routes: Tier 3+
					r.With(middleware.RequireUnitCommander(db)).Post("/", sessionHandler.CreateSession)
					r.With(middleware.RequireUnitCommander(db)).Put("/{id}/close", sessionHandler.CloseSession)
					r.With(middleware.RequireUnitCommander(db)).Get("/{id}/export/csv", sessionHandler.ExportSessionCSV)
					r.With(middleware.RequireUnitCommander(db)).Get("/{id}/export/excel", sessionHandler.ExportSessionExcel)
					r.With(middleware.RequireUnitCommander(db)).Get("/{id}/export/pdf", sessionHandler.ExportSessionPDF)
					r.With(middleware.RequireUnitCommander(db)).Post("/{id}/attendance/manual", attendanceHandler.ManualMarkAttendance)
					r.With(middleware.RequireUnitCommander(db)).Delete("/{id}/attendance/{userId}", attendanceHandler.RemoveAttendance)

					// Delete: superadmin only
					r.With(middleware.RequireSuperadmin(db)).Delete("/{id}", sessionHandler.DeleteSession)

					// Custom participant sessions: Tier 3+
					r.With(middleware.RequireUnitCommander(db)).Post("/custom/preview", sessionHandler.PreviewCustomSession)
					r.With(middleware.RequireUnitCommander(db)).Post("/custom/create", sessionHandler.CreateCustomSession)
				})

				// Reports routes: Tier 2+
				r.Route("/reports", func(r chi.Router) {
					r.Use(middleware.RequireBatteryNCO(db))
					reportsHandler := handlers.NewReportsHandler(db)
					r.Get("/sessions/{id}/analytics", reportsHandler.GetSessionAnalytics)
					r.Get("/sessions/{id}/missing", reportsHandler.GetMissingUsers)
					r.Get("/user/{userId}", reportsHandler.GetUserReport)
					r.Get("/battery/{battery}", reportsHandler.GetBatteryReport)
				})

				// Admin routes: superadmin only
				r.Route("/admin", func(r chi.Router) {
					r.Use(middleware.RequireSuperadmin(db))
					adminHandler := handlers.NewAdminHandler(db)
					r.Post("/users/bulk-upload", adminHandler.BulkUploadUsers)
					r.Post("/users/bulk-create", adminHandler.BulkCreateUsers)
					// Registration approval
					r.Get("/registrations", adminHandler.ListPendingRegistrations)
					r.Post("/registrations/{id}/approve", adminHandler.ApproveRegistration)
					r.Post("/registrations/{id}/reject", adminHandler.RejectRegistration)
				})

				// Status routes: Tier 3+
				r.Route("/statuses", func(r chi.Router) {
					r.Use(middleware.RequireUnitCommander(db))
					statusHandler := handlers.NewStatusHandler(db, sseHub)
					r.Get("/", statusHandler.ListStatuses)
					r.Post("/", statusHandler.CreateStatus)
					r.Get("/{id}", statusHandler.GetStatus)
					r.Put("/{id}", statusHandler.UpdateStatus)
					r.Delete("/{id}", statusHandler.DeleteStatus)
				})
			})

			// Webhook routes (verified via signature)
			webhookHandler := handlers.NewWebhookHandler(db)
			r.Post("/webhooks/polar", webhookHandler.PolarWebhook)
		})
	})

	// Server configuration
	port := getEnv("PORT", "8080")
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", port),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 310 * time.Second, // covers 5-min import routes
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		log.Printf("Server starting on port %s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Server shutting down...")

	// Shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server forced to shutdown: %v", err)
	}

	log.Println("Server exited")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// newAgentParser builds the Anthropic-backed parser used by the
// document-import admin route. Returns nil (with a log line) when
// ANTHROPIC_API_KEY is unset so the rest of the app keeps working;
// the import handler converts a nil parser to a clear 503 response.
func newAgentParser() agent.Parser {
	client, err := agent.NewHTTPClient()
	if err != nil {
		log.Printf("Anthropic agent disabled: %v (set ANTHROPIC_API_KEY to enable)", err)
		return nil
	}
	return agent.NewParser(client)
}
