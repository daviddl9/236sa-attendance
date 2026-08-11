package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/services/matching"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type RegistrationCandidate struct {
	ID              string   `json:"id"`
	FullName        string   `json:"fullName"`
	Rank            string   `json:"rank"`
	Battery         string   `json:"battery"`
	Score           int      `json:"score"`
	Strong          bool     `json:"strong"`
	MismatchReasons []string `json:"mismatchReasons"`
	AlreadyClaimed  bool     `json:"alreadyClaimed"`
	ClaimedBy       string   `json:"claimedBy,omitempty"`
	Selectable      bool     `json:"selectable"`
	Preselected     bool     `json:"preselected"`
}

// ListRegistrationCandidates ranks existing roster rows for a pending signup.
func (h *AdminHandler) ListRegistrationCandidates(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	registrationID := chi.URLParam(r, "id")
	var registration matching.Registration
	if err := h.db.Pool.QueryRow(ctx, `
		SELECT claimed_name, claimed_rank, claimed_battery
		FROM pending_registration WHERE id = $1
	`, registrationID).Scan(&registration.Name, &registration.Rank, &registration.Battery); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "Registration not found", http.StatusNotFound)
			return
		}
		http.Error(w, "Failed to fetch registration", http.StatusInternalServerError)
		return
	}

	rows, err := h.db.Pool.Query(ctx, `
		SELECT id, COALESCE("full_name", ''), COALESCE(rank, ''), COALESCE(battery, ''), username
		FROM "user"
	`)
	if err != nil {
		http.Error(w, "Failed to fetch roster", http.StatusInternalServerError)
		return
	}
	defer rows.Close()
	roster := make([]matching.RosterRow, 0)
	for rows.Next() {
		var row matching.RosterRow
		if err := rows.Scan(&row.ID, &row.Name, &row.Rank, &row.Battery, &row.Username); err != nil {
			http.Error(w, "Failed to fetch roster", http.StatusInternalServerError)
			return
		}
		roster = append(roster, row)
	}
	if err := rows.Err(); err != nil {
		http.Error(w, "Failed to fetch roster", http.StatusInternalServerError)
		return
	}

	candidates := matching.RankCandidates(registration, roster)
	response := make([]RegistrationCandidate, len(candidates))
	for i, candidate := range candidates {
		reasons := make([]string, 0, 2)
		if candidate.RankMismatch {
			reasons = append(reasons, fmt.Sprintf("rank differs — roster %s, entered %s", candidate.Rank, registration.Rank))
		}
		if candidate.BatteryMismatch {
			reasons = append(reasons, fmt.Sprintf("battery differs — roster %s, entered %s", candidate.Battery, registration.Battery))
		}
		response[i] = RegistrationCandidate{
			ID: candidate.ID, FullName: candidate.Name, Rank: candidate.Rank, Battery: candidate.Battery,
			Score: candidate.Score, Strong: candidate.Strong, MismatchReasons: reasons, AlreadyClaimed: candidate.AlreadyClaimed,
			ClaimedBy: candidate.ClaimedUsername, Selectable: candidate.Selectable, Preselected: candidate.Preselected,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"candidates": response})
}
