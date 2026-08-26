package models

import "time"

// Session types. Ordinary attendance sessions carry a roster derived from
// their scope; "out" sessions start empty and soldiers enrol themselves by
// scanning at the gate.
const (
	SessionTypeAttendance = "attendance"
	SessionTypeOut        = "out"
)

// Out session sub-types.
const (
	OutSubtypeNightsOut = "nights_out" // out for the evening, back the same night
	OutSubtypeStayOut   = "stay_out"   // out overnight, back for first parade
	OutSubtypeOffPass   = "off_pass"   // out for a period, back the same day
)

// ValidOutSubtypes lists every accepted Out sub-type.
var ValidOutSubtypes = []string{
	OutSubtypeNightsOut,
	OutSubtypeStayOut,
	OutSubtypeOffPass,
}

// OutSubtypeDisplayNames maps internal sub-types to display names.
var OutSubtypeDisplayNames = map[string]string{
	OutSubtypeNightsOut: "Nights Out",
	OutSubtypeStayOut:   "Stay Out",
	OutSubtypeOffPass:   "Off Pass",
}

// IsValidOutSubtype reports whether subtype is an accepted Out sub-type.
func IsValidOutSubtype(subtype string) bool {
	for _, valid := range ValidOutSubtypes {
		if subtype == valid {
			return true
		}
	}
	return false
}

// OutSubtypeDisplayName returns the human-readable sub-type name, falling back
// to the raw value so an unknown sub-type is still legible.
func OutSubtypeDisplayName(subtype string) string {
	if name, ok := OutSubtypeDisplayNames[subtype]; ok {
		return name
	}
	return subtype
}

// Movement directions recorded against an Out session.
const (
	DirectionOut = "out"
	DirectionIn  = "in"
)

// OutSession is an attendance_session row of type 'out'. It is a distinct view
// rather than a widening of AttendanceSession: an Out session has no scope,
// no batteries and no end time, and keeping the types apart leaves every
// existing attendance query untouched.
type OutSession struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Subtype          string     `json:"subtype"`
	SubtypeLabel     string     `json:"subtypeLabel"`
	QRCode           string     `json:"qrCode,omitempty"`
	QRCodeSecret     string     `json:"-"` // Never serialize secret
	Status           string     `json:"status"`
	CreatedBy        string     `json:"createdBy"`
	StartTime        time.Time  `json:"startTime"`
	ExpectedReturnAt time.Time  `json:"expectedReturnAt"`
	ClosedAt         *time.Time `json:"closedAt,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

// OutMovement is one append-only gate movement. Voided rows are retained for
// the audit trail and excluded from current-state derivation.
type OutMovement struct {
	ID         string     `json:"id"`
	SessionID  string     `json:"sessionId"`
	UserID     string     `json:"userId"`
	Direction  string     `json:"direction"`
	OccurredAt time.Time  `json:"occurredAt"`
	Method     string     `json:"method"`
	RecordedBy *string    `json:"recordedBy,omitempty"` // set for manual records
	VoidedAt   *time.Time `json:"voidedAt,omitempty"`
	VoidedBy   *string    `json:"voidedBy,omitempty"`
	VoidReason *string    `json:"voidReason,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
}
