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

				// User management routes (commander+ or superadmin)
				r.Route("/users", func(r chi.Router) {
					r.Use(middleware.RequireCommander(db)) // Allows commanders (3SG+) and superadmins
					r.Get("/", userHandler.ListUsers)
					r.Get("/bulk/count", userHandler.BulkDeleteCount)
					r.Delete("/bulk", userHandler.BulkDelete)
					r.Get("/{id}", userHandler.GetUser)
					r.Put("/{id}", userHandler.UpdateUser)
					r.Delete("/{id}", userHandler.DeleteUser)
				})

				// Attendance routes
				r.Post("/attendance/mark", attendanceHandler.MarkAttendance)

				// Session routes (commander+)
				r.Route("/sessions", func(r chi.Router) {
					sessionHandler := handlers.NewSessionHandler(db, sseHub)
					r.Use(middleware.RequireCommander(db))
					r.Post("/", sessionHandler.CreateSession)
					r.Get("/", sessionHandler.ListSessions)
					r.Get("/active", sessionHandler.GetActiveSessions)
					r.Get("/{id}", sessionHandler.GetSession)
					r.Put("/{id}/close", sessionHandler.CloseSession)
					r.Get("/{id}/export/csv", sessionHandler.ExportSessionCSV)
					r.Get("/{id}/export/excel", sessionHandler.ExportSessionExcel)
					r.Get("/{id}/export/pdf", sessionHandler.ExportSessionPDF)

					// Attendance marking for sessions (commander+)
					r.Post("/{id}/attendance/manual", attendanceHandler.ManualMarkAttendance)
					r.Delete("/{id}/attendance/{userId}", attendanceHandler.RemoveAttendance)

					// Superadmin-only session routes
					r.Group(func(r chi.Router) {
						r.Use(middleware.RequireSuperadmin(db))
						r.Delete("/{id}", sessionHandler.DeleteSession)
					})
				})

				// Reports routes (commander+)
				r.Route("/reports", func(r chi.Router) {
					r.Use(middleware.RequireCommander(db))
					reportsHandler := handlers.NewReportsHandler(db)
					r.Get("/sessions/{id}/analytics", reportsHandler.GetSessionAnalytics)
					r.Get("/sessions/{id}/missing", reportsHandler.GetMissingUsers)
					r.Get("/user/{userId}", reportsHandler.GetUserReport)
					r.Get("/battery/{battery}", reportsHandler.GetBatteryReport)
				})

				// Admin routes (superadmin only)
				r.Route("/admin", func(r chi.Router) {
					r.Use(middleware.RequireSuperadmin(db))
					adminHandler := handlers.NewAdminHandler(db)
					r.Post("/users/bulk-upload", adminHandler.BulkUploadUsers)
				})

				// Status routes (superadmin only)
				r.Route("/statuses", func(r chi.Router) {
					r.Use(middleware.RequireSuperadmin(db))
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
		WriteTimeout: 15 * time.Second,
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
