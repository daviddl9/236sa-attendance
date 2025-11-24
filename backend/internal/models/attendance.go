package models

import "time"

// Session types
const (
	SessionTypeFirstParade     = "first_parade"
	SessionTypeMorningFormation = "morning_formation"
	SessionTypeCustom          = "custom"
)

// Session scopes
const (
	SessionScopeUnitWide      = "unit_wide"
	SessionScopeBatterySpecific = "battery_specific"
)

// Session statuses
const (
	SessionStatusActive = "active"
	SessionStatusClosed = "closed"
)

// Attendance marking methods
const (
	MarkingMethodQRScan = "qr_scan"
	MarkingMethodManual = "manual"
)

type AttendanceSession struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	SessionType   string    `json:"sessionType"`
	QRCode        string    `json:"qrCode"`
	QRCodeSecret  string    `json:"-"` // Never serialize secret
	Scope         string    `json:"scope"`
	Batteries     []string  `json:"batteries"`
	Status        string    `json:"status"`
	CreatedBy     string    `json:"createdBy"`
	StartTime     time.Time `json:"startTime"`
	EndTime       *time.Time `json:"endTime,omitempty"`
	ClosedAt      *time.Time `json:"closedAt,omitempty"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
}

type AttendanceRecord struct {
	ID            string    `json:"id"`
	SessionID     string    `json:"sessionId"`
	UserID        string    `json:"userId"`
	MarkedAt      time.Time `json:"markedAt"`
	MarkingMethod string    `json:"markingMethod"`
	MarkedBy      *string   `json:"markedBy,omitempty"` // User ID of commander if manual
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

