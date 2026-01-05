package sse

import "time"

// Event types for SSE
const (
	EventTypeAttendanceMarked  = "attendance_marked"
	EventTypeAttendanceRemoved = "attendance_removed"
	EventTypeSessionClosed     = "session_closed"
	EventTypeStatusChanged     = "status_changed"
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
