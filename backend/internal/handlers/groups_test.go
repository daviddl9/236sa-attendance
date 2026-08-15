package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
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
