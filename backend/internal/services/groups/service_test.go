package groups

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/database"
)

func openGroupServiceDB(t *testing.T) (*database.DB, string) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run the group service tests")
	}
	db, err := database.NewPostgresDB(url)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	prefix := fmt.Sprintf("grp-%d", os.Getpid())
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM attendance_session WHERE id LIKE $1`, prefix+"-%")
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM participant_group WHERE created_by LIKE $1`, prefix+"-%")
		_, _ = db.Pool.Exec(context.Background(), `DELETE FROM "user" WHERE id LIKE $1`, prefix+"-%")
		db.Close()
	})
	return db, prefix
}

func seedGroupUser(t *testing.T, db *database.DB, id, name string) {
	t.Helper()
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO "user" (id, "full_name", rank, battery, extras, verified, "is_superadmin", "createdAt", "updatedAt")
		VALUES ($1, $2, 'PTE', 'Alpha', '{}'::jsonb, true, false, NOW(), NOW())
	`, id, name)
	if err != nil {
		t.Fatalf("seed user %s: %v", id, err)
	}
}

func TestCreateValidatesAndPersistsGroup(t *testing.T) {
	db, prefix := openGroupServiceDB(t)
	creator := prefix + "-creator"
	seedGroupUser(t, db, creator, "CREATOR")
	m1 := prefix + "-m1"
	m2 := prefix + "-m2"
	seedGroupUser(t, db, m1, "MEMBER ONE")
	seedGroupUser(t, db, m2, "MEMBER TWO")

	svc := NewService(db)

	// Empty name / no members are rejected.
	if _, err := svc.Create(context.Background(), "  ", []string{m1}, creator); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("Create(empty name) error = %v, want ErrInvalidRequest", err)
	}
	if _, err := svc.Create(context.Background(), "Advance Party", nil, creator); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("Create(no members) error = %v, want ErrInvalidRequest", err)
	}

	group, err := svc.Create(context.Background(), "Advance Party", []string{m1, m2, m1}, creator)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if group.ID == "" || group.Name != "Advance Party" || group.CreatedBy != creator {
		t.Fatalf("Create() returned unexpected group: %+v", group)
	}
	if group.MemberCount == nil || *group.MemberCount != 3 {
		t.Fatalf("MemberCount = %v, want 3 (dedupe not applied at insert count)", group.MemberCount)
	}

	// Duplicate name for the same creator is rejected.
	if _, err := svc.Create(context.Background(), "advance party", []string{m1}, creator); !errors.Is(err, ErrDuplicateName) {
		t.Fatalf("Create(duplicate name) error = %v, want ErrDuplicateName", err)
	}

	// List returns the group with its count.
	groups, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	found := false
	for _, g := range groups {
		if g.ID == group.ID {
			found = true
			if g.MemberCount == nil || *g.MemberCount != 2 {
				t.Fatalf("List() MemberCount = %v, want 2 (deduped by PK)", g.MemberCount)
			}
		}
	}
	if !found {
		t.Fatalf("List() did not include created group")
	}

	// Get returns member IDs (deduped).
	got, members, err := svc.Get(context.Background(), group.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Name != "Advance Party" {
		t.Fatalf("Get() Name = %q, want Advance Party", got.Name)
	}
	if len(members) != 2 {
		t.Fatalf("Get() members = %v, want 2", members)
	}

	// Delete removes it.
	if err := svc.Delete(context.Background(), group.ID); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, _, err := svc.Get(context.Background(), group.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get(after delete) error = %v, want ErrNotFound", err)
	}
	if err := svc.Delete(context.Background(), group.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Delete(missing) error = %v, want ErrNotFound", err)
	}
}

func TestSaveSessionAsGroup(t *testing.T) {
	db, prefix := openGroupServiceDB(t)
	creator := prefix + "-creator"
	seedGroupUser(t, db, creator, "CREATOR")
	m1 := prefix + "-m1"
	m2 := prefix + "-m2"
	seedGroupUser(t, db, m1, "MEMBER ONE")
	seedGroupUser(t, db, m2, "MEMBER TWO")

	svc := NewService(db)

	// A session with no participants cannot be saved as a group.
	if _, err := svc.SaveSessionAsGroup(context.Background(), prefix+"-nosuch", "Group", creator); err == nil {
		t.Fatalf("SaveSessionAsGroup(no participants) succeeded; want error")
	}

	// Seed a session with participants.
	sessionID := prefix + "-session"
	qr := prefix + "-qr"
	dl := prefix + "-dl"
	if _, err := db.Pool.Exec(context.Background(), `
		INSERT INTO attendance_session (id, name, qr_code, qr_code_secret, scope, batteries, status, created_by, start_time, deeplink_code, "createdAt", "updatedAt")
		VALUES ($1, 'Parade', $3, 'secret', 'custom_list', '{}', 'active', $2, NOW(), $4, NOW(), NOW())
	`, sessionID, creator, qr, dl); err != nil {
		t.Fatalf("seed session: %v", err)
	}
	for _, m := range []string{m1, m2} {
		if _, err := db.Pool.Exec(context.Background(), `
			INSERT INTO session_participants (session_id, user_id) VALUES ($1, $2)
		`, sessionID, m); err != nil {
			t.Fatalf("seed participant: %v", err)
		}
	}

	group, err := svc.SaveSessionAsGroup(context.Background(), sessionID, "Parade Group", creator)
	if err != nil {
		t.Fatalf("SaveSessionAsGroup() error = %v", err)
	}
	_, members, err := svc.Get(context.Background(), group.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("group members = %v, want 2", members)
	}
}

func TestSetMembersReplacesMembers(t *testing.T) {
	db, prefix := openGroupServiceDB(t)
	creator := prefix + "-creator"
	seedGroupUser(t, db, creator, "CREATOR")
	m1 := prefix + "-m1"
	m2 := prefix + "-m2"
	m3 := prefix + "-m3"
	seedGroupUser(t, db, m1, "MEMBER ONE")
	seedGroupUser(t, db, m2, "MEMBER TWO")
	seedGroupUser(t, db, m3, "MEMBER THREE")

	svc := NewService(db)

	// Seed a group with two members.
	group, err := svc.Create(context.Background(), "Replace Me", []string{m1, m2}, creator)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Unknown group -> ErrNotFound.
	if _, err := svc.SetMembers(context.Background(), prefix+"-missing", []string{m1}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetMembers(missing group) error = %v, want ErrNotFound", err)
	}

	// Replace with a deduped list: m3 + duplicate m2 + m2-again.
	count, err := svc.SetMembers(context.Background(), group.ID, []string{m3, m2, m2})
	if err != nil {
		t.Fatalf("SetMembers() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("SetMembers() count = %d, want 2", count)
	}

	_, members, err := svc.Get(context.Background(), group.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("Get() members = %v, want 2 after replace", members)
	}
	want := map[string]bool{m2: true, m3: true}
	for _, uid := range members {
		if !want[uid] {
			t.Fatalf("Get() member %q not in expected set %v", uid, want)
		}
	}

	// Replace with an empty list clears the group.
	emptyCount, err := svc.SetMembers(context.Background(), group.ID, nil)
	if err != nil {
		t.Fatalf("SetMembers(empty) error = %v", err)
	}
	if emptyCount != 0 {
		t.Fatalf("SetMembers(empty) count = %d, want 0", emptyCount)
	}
	_, members, err = svc.Get(context.Background(), group.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(members) != 0 {
		t.Fatalf("Get() members = %v, want empty after clear", members)
	}
}

func TestRemoveMember(t *testing.T) {
	db, prefix := openGroupServiceDB(t)
	creator := prefix + "-creator"
	seedGroupUser(t, db, creator, "CREATOR")
	m1 := prefix + "-m1"
	m2 := prefix + "-m2"
	seedGroupUser(t, db, m1, "MEMBER ONE")
	seedGroupUser(t, db, m2, "MEMBER TWO")

	svc := NewService(db)
	group, err := svc.Create(context.Background(), "Remove Test", []string{m1, m2}, creator)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	// Unknown group -> ErrNotFound.
	if err := svc.RemoveMember(context.Background(), prefix+"-missing", m1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("RemoveMember(missing group) error = %v, want ErrNotFound", err)
	}

	// Removing a member that is not in the group is a no-op.
	if err := svc.RemoveMember(context.Background(), group.ID, prefix+"-notamember"); err != nil {
		t.Fatalf("RemoveMember(non-member) error = %v, want nil", err)
	}

	// Remove an actual member.
	if err := svc.RemoveMember(context.Background(), group.ID, m1); err != nil {
		t.Fatalf("RemoveMember(m1) error = %v", err)
	}
	_, members, err := svc.Get(context.Background(), group.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if len(members) != 1 || members[0] != m2 {
		t.Fatalf("Get() members = %v, want [%s]", members, m2)
	}
}

func TestMembersWithDetailsOrdersByName(t *testing.T) {
	db, prefix := openGroupServiceDB(t)
	creator := prefix + "-creator"
	seedGroupUser(t, db, creator, "CREATOR")
	zUser := prefix + "-z"
	aUser := prefix + "-a"
	seedUserWithDetails(t, db, zUser, "ZULKIPLI", "CPT", "HQ")
	seedUserWithDetails(t, db, aUser, "ABU BAKAR", "PTE", "Alpha")

	svc := NewService(db)
	group, err := svc.Create(context.Background(), "Detail Order", []string{zUser, aUser}, creator)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	members, err := svc.MembersWithDetails(context.Background(), group.ID)
	if err != nil {
		t.Fatalf("MembersWithDetails() error = %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("MembersWithDetails() count = %d, want 2", len(members))
	}
	if members[0].UserID != aUser || members[0].FullName != "ABU BAKAR" {
		t.Fatalf("members[0] = %+v, want user %s ABU BAKAR (ordered by lower name)", members[0], aUser)
	}
	if members[1].UserID != zUser {
		t.Fatalf("members[1] = %+v, want user %s", members[1], zUser)
	}
	if members[1].Rank == nil || *members[1].Rank != "CPT" {
		t.Fatalf("members[1].Rank = %v, want CPT", members[1].Rank)
	}
	if members[1].Battery == nil || *members[1].Battery != "HQ" {
		t.Fatalf("members[1].Battery = %v, want HQ", members[1].Battery)
	}

	// A group with no members returns an empty, non-nil slice.
	if err := svc.RemoveMember(context.Background(), group.ID, zUser); err != nil {
		t.Fatalf("RemoveMember() error = %v", err)
	}
	if err := svc.RemoveMember(context.Background(), group.ID, aUser); err != nil {
		t.Fatalf("RemoveMember() error = %v", err)
	}
	empty, err := svc.MembersWithDetails(context.Background(), group.ID)
	if err != nil {
		t.Fatalf("MembersWithDetails(empty) error = %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Fatalf("MembersWithDetails(empty) = %v, want empty non-nil slice", empty)
	}
}

func seedUserWithDetails(t *testing.T, db *database.DB, id, name, rank, battery string) {
	t.Helper()
	_, err := db.Pool.Exec(context.Background(), `
		INSERT INTO "user" (id, "full_name", rank, battery, extras, verified, "is_superadmin", "createdAt", "updatedAt")
		VALUES ($1, $2, $3, $4, '{}'::jsonb, true, false, NOW(), NOW())
	`, id, name, rank, battery)
	if err != nil {
		t.Fatalf("seed user %s: %v", id, err)
	}
}
