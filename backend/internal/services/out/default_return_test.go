package out

import (
	"testing"
	"time"

	"github.com/davidlivingston/go-nextjs-starter/backend/internal/models"
	"github.com/davidlivingston/go-nextjs-starter/backend/internal/timeutil"
)

// sgt builds an SGT wall-clock instant for readable expectations.
func sgt(year int, month time.Month, day, hour, minute int) time.Time {
	return time.Date(year, month, day, hour, minute, 0, 0, timeutil.SGT)
}

func TestDefaultReturnAt(t *testing.T) {
	for _, tc := range []struct {
		name    string
		subtype string
		now     time.Time
		want    time.Time
	}{
		{
			name:    "nights out returns the same evening",
			subtype: models.OutSubtypeNightsOut,
			now:     sgt(2026, time.August, 21, 18, 30),
			want:    sgt(2026, time.August, 21, 23, 0),
		},
		{
			name:    "off pass returns the same evening",
			subtype: models.OutSubtypeOffPass,
			now:     sgt(2026, time.August, 21, 14, 0),
			want:    sgt(2026, time.August, 21, 23, 0),
		},
		{
			name:    "stay out returns the next morning",
			subtype: models.OutSubtypeStayOut,
			now:     sgt(2026, time.August, 21, 18, 30),
			want:    sgt(2026, time.August, 22, 7, 0),
		},
		{
			name:    "same-day default rolls forward when opened after 23:00",
			subtype: models.OutSubtypeNightsOut,
			now:     sgt(2026, time.August, 21, 23, 30),
			want:    sgt(2026, time.August, 22, 23, 0),
		},
		{
			name:    "same-day default rolls forward when opened exactly at 23:00",
			subtype: models.OutSubtypeNightsOut,
			now:     sgt(2026, time.August, 21, 23, 0),
			want:    sgt(2026, time.August, 22, 23, 0),
		},
		{
			name:    "stay out crosses a month boundary",
			subtype: models.OutSubtypeStayOut,
			now:     sgt(2026, time.August, 31, 20, 0),
			want:    sgt(2026, time.September, 1, 7, 0),
		},
		{
			name:    "stay out crosses a year boundary",
			subtype: models.OutSubtypeStayOut,
			now:     sgt(2026, time.December, 31, 20, 0),
			want:    sgt(2027, time.January, 1, 7, 0),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := DefaultReturnAt(tc.subtype, tc.now)
			if !got.Equal(tc.want) {
				t.Fatalf("DefaultReturnAt(%q, %s) = %s; want %s",
					tc.subtype, tc.now.Format(time.RFC3339),
					got.Format(time.RFC3339), tc.want.Format(time.RFC3339))
			}
		})
	}
}

// The production server runs in UTC. A late-evening UTC instant is already the
// following day in Singapore, so the default must be computed from the SGT
// calendar date rather than the server's.
func TestDefaultReturnAtUsesSingaporeCalendarDate(t *testing.T) {
	// 2026-08-21T16:30:00Z is 2026-08-22T00:30 in SGT.
	utcNow := time.Date(2026, time.August, 21, 16, 30, 0, 0, time.UTC)

	nightsOut := DefaultReturnAt(models.OutSubtypeNightsOut, utcNow)
	if want := sgt(2026, time.August, 22, 23, 0); !nightsOut.Equal(want) {
		t.Fatalf("nights out from UTC instant = %s; want %s",
			nightsOut.Format(time.RFC3339), want.Format(time.RFC3339))
	}

	stayOut := DefaultReturnAt(models.OutSubtypeStayOut, utcNow)
	if want := sgt(2026, time.August, 23, 7, 0); !stayOut.Equal(want) {
		t.Fatalf("stay out from UTC instant = %s; want %s",
			stayOut.Format(time.RFC3339), want.Format(time.RFC3339))
	}
}

func TestDefaultReturnAtIsAlwaysInTheFuture(t *testing.T) {
	// Walk every minute of a day and assert the same-day default never lands
	// in the past, which would make enrollees overdue the moment they scan.
	start := sgt(2026, time.August, 21, 0, 0)
	for minute := 0; minute < 24*60; minute++ {
		now := start.Add(time.Duration(minute) * time.Minute)
		for _, subtype := range models.ValidOutSubtypes {
			if got := DefaultReturnAt(subtype, now); !got.After(now) {
				t.Fatalf("DefaultReturnAt(%q, %s) = %s; want a future instant",
					subtype, now.Format(time.RFC3339), got.Format(time.RFC3339))
			}
		}
	}
}
