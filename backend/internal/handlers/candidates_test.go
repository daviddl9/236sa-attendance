package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/go-chi/chi/v5"
)

func TestCandidatesEndpointRequiresCommander(t *testing.T) {
	r := chi.NewRouter()
	r.Route("/api/admin/registrations", func(r chi.Router) {
		r.Use(middleware.Auth(nil))
		r.Use(middleware.LoadUser(nil))
		r.Use(middleware.RequireSuperadmin(nil))
		r.Get("/{id}/candidates", NewAdminHandler(nil).ListRegistrationCandidates)
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/registrations/secret/candidates", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "secret") || strings.Contains(rec.Body.String(), "TAN") {
		t.Fatalf("unauthenticated response leaks a name or identifier: %q", rec.Body.String())
	}
}
