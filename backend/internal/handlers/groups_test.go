package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/middleware"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/go-chi/chi/v5"
)

func withGroupUser(req *http.Request, user *models.User, id string) *http.Request {
	return withSessionUser(req, user, id)
}

func TestGroupHandlerCRUDAndSessionFlows(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	creatorID := prefix + "-creator"
	seedUser(t, db, creatorID, "GROUP CREATOR", "CPT", "HQ", prefix+"-creator", true)
	m1 := prefix + "-m1"
	m2 := prefix + "-m2"
	seedUser(t, db, m1, "MEMBER ONE", "PTE", "Alpha", prefix+"-m1", true)
	seedUser(t, db, m2, "MEMBER TWO", "PTE", "Alpha", prefix+"-m2", true)

	handler := NewGroupHandler(db)
	creator := sessionTestUser(creatorID, models.RankCPT)

	// Create a group.
	createBody := `{"name":"Advance Party","participantIds":["` + m1 + `","` + m2 + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/groups", bytes.NewBufferString(createBody))
	req = withGroupUser(req, creator, "")
	rec := httptest.NewRecorder()
	handler.CreateGroup(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("CreateGroup status = %d: %s", rec.Code, rec.Body.String())
	}
	var created GroupResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created group: %v", err)
	}
	if created.ID == "" || created.Name != "Advance Party" {
		t.Fatalf("unexpected created group: %+v", created)
	}
	if created.MemberCount == nil || *created.MemberCount != 2 {
		t.Fatalf("created MemberCount = %v, want 2", created.MemberCount)
	}

	// Duplicate name is rejected with 409.
	dupBody := `{"name":"advance party","participantIds":["` + m1 + `"]}`
	req = httptest.NewRequest(http.MethodPost, "/api/groups", bytes.NewBufferString(dupBody))
	req = withGroupUser(req, creator, "")
	rec = httptest.NewRecorder()
	handler.CreateGroup(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("CreateGroup(dup) status = %d, want 409: %s", rec.Code, rec.Body.String())
	}

	// List groups.
	req = httptest.NewRequest(http.MethodGet, "/api/groups", nil)
	req = withGroupUser(req, creator, "")
	rec = httptest.NewRecorder()
	handler.ListGroups(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("ListGroups status = %d: %s", rec.Code, rec.Body.String())
	}
	var listResp struct {
		Groups []models.ParticipantGroup `json:"groups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	found := false
	for _, g := range listResp.Groups {
		if g.ID == created.ID {
			found = true
			if g.MemberCount == nil || *g.MemberCount != 2 {
				t.Fatalf("ListGroups MemberCount = %v, want 2", g.MemberCount)
			}
		}
	}
	if !found {
		t.Fatalf("ListGroups did not include created group %s: %+v", created.ID, listResp.Groups)
	}

	// Get group with member IDs.
	req = httptest.NewRequest(http.MethodGet, "/api/groups/"+created.ID, nil)
	req = withGroupUser(req, creator, created.ID)
	rec = httptest.NewRecorder()
	handler.GetGroup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GetGroup status = %d: %s", rec.Code, rec.Body.String())
	}
	var got GroupResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal get: %v", err)
	}
	if len(got.MemberIDs) != 2 {
		t.Fatalf("GetGroup memberIds = %v, want 2", got.MemberIDs)
	}

	// Create a session from the group.
	sessBody := `{"name":"Advance Party Session"}`
	req = httptest.NewRequest(http.MethodPost, "/api/groups/"+created.ID+"/sessions", bytes.NewBufferString(sessBody))
	req = withGroupUser(req, creator, created.ID)
	rec = httptest.NewRecorder()
	handler.CreateSessionFromGroup(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("CreateSessionFromGroup status = %d: %s", rec.Code, rec.Body.String())
	}
	var sessResp SessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &sessResp); err != nil {
		t.Fatalf("unmarshal session: %v", err)
	}
	if sessResp.ID == "" || sessResp.Scope != models.SessionScopeCustomList {
		t.Fatalf("unexpected session: %+v", sessResp)
	}

	// Duplicate the session.
	dupSessBody := `{"name":"Advance Party Session (copy)"}`
	req = httptest.NewRequest(http.MethodPost, "/api/sessions/"+sessResp.ID+"/duplicate", bytes.NewBufferString(dupSessBody))
	req = withGroupUser(req, creator, sessResp.ID)
	rec = httptest.NewRecorder()
	handler.DuplicateSession(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("DuplicateSession status = %d: %s", rec.Code, rec.Body.String())
	}
	var dupSess SessionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &dupSess); err != nil {
		t.Fatalf("unmarshal dup session: %v", err)
	}
	if dupSess.ID == "" || dupSess.ID == sessResp.ID {
		t.Fatalf("duplicate session id = %q, want new id", dupSess.ID)
	}

	// Save the session as a group.
	saveBody := `{"name":"Saved From Session"}`
	req = httptest.NewRequest(http.MethodPost, "/api/sessions/"+sessResp.ID+"/group", bytes.NewBufferString(saveBody))
	req = withGroupUser(req, creator, sessResp.ID)
	rec = httptest.NewRecorder()
	handler.SaveSessionAsGroup(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("SaveSessionAsGroup status = %d: %s", rec.Code, rec.Body.String())
	}
	var saved GroupResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &saved); err != nil {
		t.Fatalf("unmarshal saved group: %v", err)
	}
	if saved.Name != "Saved From Session" || saved.MemberCount == nil || *saved.MemberCount != 2 {
		t.Fatalf("saved group = %+v, want 2 members", saved)
	}

	// Delete the original group.
	req = httptest.NewRequest(http.MethodDelete, "/api/groups/"+created.ID, nil)
	req = withGroupUser(req, creator, created.ID)
	rec = httptest.NewRecorder()
	handler.DeleteGroup(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("DeleteGroup status = %d: %s", rec.Code, rec.Body.String())
	}

	// Get the deleted group -> 404.
	req = httptest.NewRequest(http.MethodGet, "/api/groups/"+created.ID, nil)
	req = withGroupUser(req, creator, created.ID)
	rec = httptest.NewRecorder()
	handler.GetGroup(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("GetGroup(after delete) status = %d, want 404", rec.Code)
	}
}

func TestGroupMemberManagement(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	creatorID := prefix + "-creator"
	seedUser(t, db, creatorID, "GROUP CREATOR", "CPT", "HQ", prefix+"-creator", true)
	m1 := prefix + "-m1"
	m2 := prefix + "-m2"
	m3 := prefix + "-m3"
	seedUser(t, db, m1, "MEMBER ONE", "PTE", "Alpha", prefix+"-m1", true)
	seedUser(t, db, m2, "MEMBER TWO", "PTE", "Alpha", prefix+"-m2", true)
	seedUser(t, db, m3, "MEMBER THREE", "LCP", "Bravo", prefix+"-m3", true)

	handler := NewGroupHandler(db)
	creator := sessionTestUser(creatorID, models.RankCPT)

	createBody := `{"name":"Ops Group","participantIds":["` + m1 + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/groups", bytes.NewBufferString(createBody))
	req = withGroupUser(req, creator, "")
	rec := httptest.NewRecorder()
	handler.CreateGroup(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("CreateGroup status = %d: %s", rec.Code, rec.Body.String())
	}
	var created GroupResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal created group: %v", err)
	}

	// GetGroup returns member details (name/rank/battery).
	req = httptest.NewRequest(http.MethodGet, "/api/groups/"+created.ID, nil)
	req = withGroupUser(req, creator, created.ID)
	rec = httptest.NewRecorder()
	handler.GetGroup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GetGroup status = %d: %s", rec.Code, rec.Body.String())
	}
	var got GroupResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal get: %v", err)
	}
	if len(got.Members) != 1 || got.Members[0].UserID != m1 || got.Members[0].FullName != "MEMBER ONE" {
		t.Fatalf("GetGroup members = %+v, want [%s MEMBER ONE]", got.Members, m1)
	}
	if got.Members[0].Rank == nil || *got.Members[0].Rank != "PTE" {
		t.Fatalf("member rank = %v, want PTE", got.Members[0].Rank)
	}
	if got.Members[0].Battery == nil || *got.Members[0].Battery != "Alpha" {
		t.Fatalf("member battery = %v, want Alpha", got.Members[0].Battery)
	}

	// SetMembers replaces the member list (m3 + m2, m1 dropped).
	setBody := `{"memberIds":["` + m3 + `","` + m2 + `"]}`
	req = httptest.NewRequest(http.MethodPut, "/api/groups/"+created.ID+"/members", bytes.NewBufferString(setBody))
	req = withGroupUser(req, creator, created.ID)
	rec = httptest.NewRecorder()
	handler.SetMembers(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("SetMembers status = %d: %s", rec.Code, rec.Body.String())
	}
	var setResp struct {
		MemberCount int `json:"memberCount"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &setResp); err != nil {
		t.Fatalf("unmarshal set response: %v", err)
	}
	if setResp.MemberCount != 2 {
		t.Fatalf("SetMembers memberCount = %d, want 2", setResp.MemberCount)
	}

	// RemoveMember drops m2 (route context carries both id and userId params).
	req = httptest.NewRequest(http.MethodDelete, "/api/groups/"+created.ID+"/members/"+m2, nil)
	rc := chi.NewRouteContext()
	rc.URLParams.Add("id", created.ID)
	rc.URLParams.Add("userId", m2)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, creator))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rc))
	rec = httptest.NewRecorder()
	handler.RemoveMember(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("RemoveMember status = %d: %s", rec.Code, rec.Body.String())
	}

	// Remaining member is m3.
	req = httptest.NewRequest(http.MethodGet, "/api/groups/"+created.ID, nil)
	req = withGroupUser(req, creator, created.ID)
	rec = httptest.NewRecorder()
	handler.GetGroup(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GetGroup status = %d: %s", rec.Code, rec.Body.String())
	}
	got = GroupResponse{}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("unmarshal get: %v", err)
	}
	if len(got.Members) != 1 || got.Members[0].UserID != m3 {
		t.Fatalf("members after remove = %+v, want [%s]", got.Members, m3)
	}

	// Unknown group -> 404 on SetMembers and RemoveMember.
	req = httptest.NewRequest(http.MethodPut, "/api/groups/"+prefix+"-missing/members", bytes.NewBufferString(`{"memberIds":["`+m1+`"]}`))
	req = withGroupUser(req, creator, prefix+"-missing")
	rec = httptest.NewRecorder()
	handler.SetMembers(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("SetMembers(missing) status = %d, want 404", rec.Code)
	}
	req = httptest.NewRequest(http.MethodDelete, "/api/groups/"+prefix+"-missing/members/"+m1, nil)
	rc = chi.NewRouteContext()
	rc.URLParams.Add("id", prefix+"-missing")
	rc.URLParams.Add("userId", m1)
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserKey, creator))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rc))
	rec = httptest.NewRecorder()
	handler.RemoveMember(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("RemoveMember(missing) status = %d, want 404", rec.Code)
	}
}

func TestGroupHandlerRename(t *testing.T) {
	db, prefix := openRegistrationDB(t)
	creatorID := prefix + "-creator"
	seedUser(t, db, creatorID, "GROUP CREATOR", "CPT", "HQ", prefix+"-creator", true)

	handler := NewGroupHandler(db)
	creator := sessionTestUser(creatorID, models.RankCPT)

	create := func(name string) GroupResponse {
		req := httptest.NewRequest(http.MethodPost, "/api/groups", bytes.NewBufferString(`{"name":"`+name+`","participantIds":[]}`))
		req = withGroupUser(req, creator, "")
		rec := httptest.NewRecorder()
		handler.CreateGroup(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("CreateGroup(%q) status = %d: %s", name, rec.Code, rec.Body.String())
		}
		var created GroupResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
			t.Fatalf("unmarshal created group: %v", err)
		}
		return created
	}

	rename := func(id, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPut, "/api/groups/"+id, bytes.NewBufferString(body))
		req = withGroupUser(req, creator, id)
		rec := httptest.NewRecorder()
		handler.RenameGroup(rec, req)
		return rec
	}

	first := create("Advance Party")
	create("Main Body")

	// Happy path: rename returns the updated group.
	rec := rename(first.ID, `{"name":"Recce Party"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("RenameGroup status = %d: %s", rec.Code, rec.Body.String())
	}
	var renamed GroupResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &renamed); err != nil {
		t.Fatalf("unmarshal renamed group: %v", err)
	}
	if renamed.ID != first.ID || renamed.Name != "Recce Party" {
		t.Fatalf("renamed group = %+v, want id %s name Recce Party", renamed, first.ID)
	}

	// Empty / whitespace-only names are rejected.
	if rec := rename(first.ID, `{"name":"   "}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("RenameGroup(blank) status = %d, want 400", rec.Code)
	}

	// Renaming onto another group's name conflicts (case-insensitive).
	if rec := rename(first.ID, `{"name":"main body"}`); rec.Code != http.StatusConflict {
		t.Fatalf("RenameGroup(dup) status = %d, want 409: %s", rec.Code, rec.Body.String())
	}

	// Unknown group -> 404.
	if rec := rename(prefix+"-missing", `{"name":"Ghost"}`); rec.Code != http.StatusNotFound {
		t.Fatalf("RenameGroup(missing) status = %d, want 404", rec.Code)
	}
}
