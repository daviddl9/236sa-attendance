package handlers

import (
	"context"
	"encoding/json"
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

func TestCandidatesIncludeSuperadminRosterRow(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	pendingID := prefix + "-pending"
	rosterID := prefix + "-commander"
	seedPending(t, db, pendingID, "commander", "COMMANDER PERSON", "CPT", "HQ")
	seedUser(t, db, rosterID, "COMMANDER PERSON", "CPT", "HQ", "", true)
	if _, err := db.Pool.Exec(context.Background(), `UPDATE "user" SET "is_superadmin" = true WHERE id = $1`, rosterID); err != nil {
		t.Fatalf("mark roster row as superadmin: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/admin/registrations/"+pendingID+"/candidates", nil)
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", pendingID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rc))
	rec := httptest.NewRecorder()
	NewAdminHandler(db).ListRegistrationCandidates(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rec.Code, rec.Body.String())
	}

	var response struct {
		Candidates []RegistrationCandidate `json:"candidates"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode candidates response: %v", err)
	}
	for _, candidate := range response.Candidates {
		if candidate.ID == rosterID {
			if !candidate.Strong {
				t.Fatal("superadmin roster row was returned as weak despite an exact match")
			}
			return
		}
	}
	t.Fatalf("superadmin roster row %q was not returned: %+v", rosterID, response.Candidates)
}
