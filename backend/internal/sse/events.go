package sse

import "time"

// Event types for SSE
const (
	EventTypeAttendanceMarked  = "attendance_marked"
	EventTypeAttendanceRemoved = "attendance_removed"
	EventTypeSessionClosed     = "session_closed"
	EventTypeStatusChanged     = "status_changed"
	EventTypeOutMovement       = "out_movement"
	EventTypeHeartbeat         = "heartbeat"
)

// Event represents an SSE event to be sent to clients
type Event struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// AttendanceMarkedPayload is sent when someone marks attendance
type AttendanceMarkedPayload struct {
	UserID        string    `json:"userId"`
	UserName      string    `json:"userName"`
	UserRank      string    `json:"userRank"`
	UserBattery   string    `json:"userBattery"`
	MarkingMethod string    `json:"markingMethod"`
	MarkedAt      time.Time `json:"markedAt"`
	PresentCount  int       `json:"presentCount"`
	TotalUsers    int       `json:"totalUsers"`
}

// AttendanceRemovedPayload is sent when attendance is removed
type AttendanceRemovedPayload struct {
	UserID       string `json:"userId"`
	PresentCount int    `json:"presentCount"`
	TotalUsers   int    `json:"totalUsers"`
}

// SessionClosedPayload is sent when a session is closed
type SessionClosedPayload struct {
	SessionID string `json:"sessionId"`
}

// StatusChangedPayload is sent when a user's status is created, updated, or deleted
type StatusChangedPayload struct {
	UserID     string `json:"userId"`
	StatusType string `json:"statusType"`
	Action     string `json:"action"` // "created", "updated", "deleted"
}

// OutMovementPayload is sent when someone scans out of, or back into, camp
// during an Out session. Counts are recomputed so a board that missed an
// earlier event still lands on the right totals.
type OutMovementPayload struct {
	UserID        string    `json:"userId"`
	UserName      string    `json:"userName,omitempty"`
	UserRank      string    `json:"userRank,omitempty"`
	Direction     string    `json:"direction"`
	MarkingMethod string    `json:"markingMethod"`
	OccurredAt    time.Time `json:"occurredAt"`
	OutCount      int       `json:"outCount"`
	ReturnedCount int       `json:"returnedCount"`
	OverdueCount  int       `json:"overdueCount"`
}
