package out

import (
	"context"
	"testing"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
)

func TestNextDirectionAlternates(t *testing.T) {
	if got := NextDirection(nil); got != models.DirectionOut {
		t.Fatalf("first scan = %q; want out", got)
	}
	out := &models.OutMovement{Direction: models.DirectionOut}
	if got := NextDirection(out); got != models.DirectionIn {
		t.Fatalf("after out = %q; want in", got)
	}
	in := &models.OutMovement{Direction: models.DirectionIn}
	if got := NextDirection(in); got != models.DirectionOut {
		t.Fatalf("after in = %q; want out (second cycle)", got)
	}
}

func TestRecordFirstScanMarksOut(t *testing.T) {
	db, prefix := openOutServiceDB(t)
	sess, soldier := seedSessionAndSoldier(t, db, prefix)

	res := mustRecord(t, db, RecordRequest{
		SessionID: sess, UserID: soldier, Method: models.MarkingMethodQRScan,
	})
	if res.Outcome != Recorded {
		t.Fatalf("outcome = %v; want Recorded", res.Outcome)
	}
	if res.Direction != models.DirectionOut {
		t.Fatalf("direction = %q; want out", res.Direction)
	}
}

func TestRecordSecondScanMarksInAndThirdGoesOutAgain(t *testing.T) {
	db, prefix := openOutServiceDB(t)
	sess, soldier := seedSessionAndSoldier(t, db, prefix)
	ctx := context.Background()
	svc := NewService(db)

	// Space the movements beyond the duplicate window.
	base := time.Now().Add(-2 * time.Hour)
	writeMovement(t, db, sess, soldier, models.DirectionOut, base)

	res, err := svc.RecordMovement(ctx, RecordRequest{
		SessionID: sess, UserID: soldier, Method: models.MarkingMethodQRScan,
	})
	if err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}
	if res.Outcome != Recorded || res.Direction != models.DirectionIn {
		t.Fatalf("second scan = %v/%q; want Recorded/in", res.Outcome, res.Direction)
	}

	// A soldier who comes back then leaves again starts a new cycle. Use a
	// separate soldier with backdated history: the scan above landed at "now",
	// so re-scanning this one would legitimately hit the duplicate guard.
	second := prefix + "-soldier2"
	seedUser(t, db, second, "PTE", models.BatteryAlpha, false)
	writeMovement(t, db, sess, second, models.DirectionOut, time.Now().Add(-3*time.Hour))
	writeMovement(t, db, sess, second, models.DirectionIn, time.Now().Add(-2*time.Hour))

	res, err = svc.RecordMovement(ctx, RecordRequest{
		SessionID: sess, UserID: second, Method: models.MarkingMethodQRScan,
	})
	if err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}
	if res.Outcome != Recorded {
		t.Fatalf("third scan outcome = %v; want Recorded", res.Outcome)
	}
	if res.Direction != models.DirectionOut {
		t.Fatalf("third scan = %q; want out (second cycle)", res.Direction)
	}
}

// A soldier who steps out and comes straight back must be able to record the
// return immediately. The confirm screen, not a timer, is what guards against
// a stray tap.
func TestRecordAllowsAnImmediateReturn(t *testing.T) {
	db, prefix := openOutServiceDB(t)
	sess, soldier := seedSessionAndSoldier(t, db, prefix)
	svc := NewService(db)
	ctx := context.Background()

	first := mustRecord(t, db, RecordRequest{
		SessionID: sess, UserID: soldier, Method: models.MarkingMethodQRScan,
	})
	if first.Direction != models.DirectionOut {
		t.Fatalf("first = %q; want out", first.Direction)
	}

	second, err := svc.RecordMovement(ctx, RecordRequest{
		SessionID: sess, UserID: soldier, Method: models.MarkingMethodQRScan,
	})
	if err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}
	if second.Outcome != Recorded {
		t.Fatalf("immediate return outcome = %v; want Recorded", second.Outcome)
	}
	if second.Direction != models.DirectionIn {
		t.Fatalf("immediate return direction = %q; want in", second.Direction)
	}

	var count int
	if err := db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM out_movement WHERE session_id = $1 AND user_id = $2`,
		sess, soldier).Scan(&count); err != nil {
		t.Fatalf("count movements: %v", err)
	}
	if count != 2 {
		t.Fatalf("movements written = %d; want 2", count)
	}
}

func TestRecordRejectsStaleExpectedDirection(t *testing.T) {
	db, prefix := openOutServiceDB(t)
	sess, soldier := seedSessionAndSoldier(t, db, prefix)
	writeMovement(t, db, sess, soldier, models.DirectionOut, time.Now().Add(-time.Hour))

	// A stale page still believes this is the soldier's first scan.
	res, err := NewService(db).RecordMovement(context.Background(), RecordRequest{
		SessionID: sess, UserID: soldier, Method: models.MarkingMethodQRScan,
		ExpectedDirection: models.DirectionOut,
	})
	if err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}
	if res.Outcome != DirectionChanged {
		t.Fatalf("outcome = %v; want DirectionChanged", res.Outcome)
	}
	if res.Direction != models.DirectionIn {
		t.Fatalf("server direction = %q; want in", res.Direction)
	}
}

func TestRecordRefusesClosedSessionAndUnverifiedUser(t *testing.T) {
	db, prefix := openOutServiceDB(t)
	sess, soldier := seedSessionAndSoldier(t, db, prefix)
	svc := NewService(db)
	ctx := context.Background()

	unverified := prefix + "-unverified"
	seedUserVerified(t, db, unverified, "PTE", models.BatteryAlpha, false, false)
	res, err := svc.RecordMovement(ctx, RecordRequest{
		SessionID: sess, UserID: unverified, Method: models.MarkingMethodQRScan,
	})
	if err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}
	if res.Outcome != NotVerified {
		t.Fatalf("unverified outcome = %v; want NotVerified", res.Outcome)
	}

	if _, err := db.Pool.Exec(ctx,
		`UPDATE attendance_session SET status='closed' WHERE id=$1`, sess); err != nil {
		t.Fatalf("close session: %v", err)
	}
	res, err = svc.RecordMovement(ctx, RecordRequest{
		SessionID: sess, UserID: soldier, Method: models.MarkingMethodQRScan,
	})
	if err != nil {
		t.Fatalf("RecordMovement: %v", err)
	}
	if res.Outcome != SessionUnavailable {
		t.Fatalf("closed session outcome = %v; want SessionUnavailable", res.Outcome)
	}
}

func TestVoidedMovementStopsCountingTowardState(t *testing.T) {
	db, prefix := openOutServiceDB(t)
	sess, soldier := seedSessionAndSoldier(t, db, prefix)
	ctx := context.Background()
	svc := NewService(db)

	writeMovement(t, db, sess, soldier, models.DirectionOut, time.Now().Add(-time.Hour))
	var id string
	if err := db.Pool.QueryRow(ctx,
		`SELECT id FROM out_movement WHERE session_id=$1 AND user_id=$2`, sess, soldier).Scan(&id); err != nil {
		t.Fatalf("read movement: %v", err)
	}
	if err := svc.Void(ctx, id, prefix+"-creator", "scanned by mistake"); err != nil {
		t.Fatalf("Void: %v", err)
	}

	latest, err := svc.LatestMovement(ctx, sess, soldier)
	if err != nil {
		t.Fatalf("LatestMovement: %v", err)
	}
	if latest != nil {
		t.Fatalf("latest = %+v; want nil after voiding the only movement", latest)
	}
	// The row itself must survive for the audit trail.
	var retained int
	if err := db.Pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM out_movement WHERE id=$1`, id).Scan(&retained); err != nil {
		t.Fatalf("count: %v", err)
	}
	if retained != 1 {
		t.Fatal("voided movement was deleted; it must be retained")
	}
	// Voiding twice is not a silent success.
	if err := svc.Void(ctx, id, prefix+"-creator", ""); err == nil {
		t.Fatal("second Void succeeded; want ErrMovementNotFound")
	}
}

func TestBoardCountsAndOverdue(t *testing.T) {
	db, prefix := openOutServiceDB(t)
	creator := prefix + "-creator"
	seedUser(t, db, creator, "SSG", models.BatteryAlpha, true)

	// An expected return already in the past makes anyone still out overdue.
	sess := seedOutSessionRow(t, db, prefix+"-sess", creator,
		models.OutSubtypeStayOut, time.Now().Add(-time.Hour))

	stillOut := prefix + "-out"
	returned := prefix + "-back"
	seedUser(t, db, stillOut, "PTE", models.BatteryAlpha, false)
	seedUser(t, db, returned, "PTE", models.BatteryBravo, false)

	writeMovement(t, db, sess, stillOut, models.DirectionOut, time.Now().Add(-3*time.Hour))
	writeMovement(t, db, sess, returned, models.DirectionOut, time.Now().Add(-3*time.Hour))
	writeMovement(t, db, sess, returned, models.DirectionIn, time.Now().Add(-30*time.Minute))

	board, err := NewService(db).GetBoard(context.Background(), sess)
	if err != nil {
		t.Fatalf("GetBoard: %v", err)
	}
	if board.OutCount != 1 || board.ReturnedCount != 1 {
		t.Fatalf("out=%d returned=%d; want 1/1", board.OutCount, board.ReturnedCount)
	}
	if board.OverdueCount != 1 {
		t.Fatalf("overdue=%d; want 1", board.OverdueCount)
	}
	if len(board.Members) != 2 {
		t.Fatalf("members=%d; want 2 — only people who scanned appear", len(board.Members))
	}
	// Overdue people sort first so the duty commander sees them immediately.
	if !board.Members[0].Overdue {
		t.Fatal("overdue member is not listed first")
	}
}

func TestCloseRequiresAcknowledgementWhilePeopleAreOut(t *testing.T) {
	db, prefix := openOutServiceDB(t)
	creator := prefix + "-creator"
	seedUser(t, db, creator, "SSG", models.BatteryAlpha, true)
	sess := seedOutSessionRow(t, db, prefix+"-sess", creator,
		models.OutSubtypeNightsOut, time.Now().Add(time.Hour))
	soldier := prefix + "-p1"
	seedUser(t, db, soldier, "PTE", models.BatteryAlpha, false)
	writeMovement(t, db, sess, soldier, models.DirectionOut, time.Now().Add(-time.Hour))

	svc := NewService(db)
	ctx := context.Background()
	if _, err := svc.Close(ctx, sess, creator, false); err != ErrStillOut {
		t.Fatalf("close without acknowledgement = %v; want ErrStillOut", err)
	}
	board, err := svc.Close(ctx, sess, creator, true)
	if err != nil {
		t.Fatalf("acknowledged close: %v", err)
	}
	if board.Session.Status != models.SessionStatusClosed {
		t.Fatalf("status = %q; want closed", board.Session.Status)
	}
	if _, err := svc.Close(ctx, sess, creator, true); err != ErrSessionNotActive {
		t.Fatalf("re-close = %v; want ErrSessionNotActive", err)
	}
}
