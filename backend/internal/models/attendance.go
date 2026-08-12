package models

import "time"

// Session scopes
const (
	SessionScopeUnitWide        = "unit_wide"
	SessionScopeBatterySpecific = "battery_specific"
	SessionScopeCustomList      = "custom_list"
)

// Session statuses
const (
	SessionStatusActive = "active"
	SessionStatusClosed = "closed"
)

// Attendance marking methods
const (
	MarkingMethodQRScan       = "qr_scan"
	MarkingMethodTelegramScan = "telegram_scan"
	MarkingMethodManual       = "manual"
)

type AttendanceSession struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	QRCode           string     `json:"qrCode,omitempty"`
	QRCodeSecret     string     `json:"-"` // Never serialize secret
	Scope            string     `json:"scope"`
	Batteries        []string   `json:"batteries"`
	Status           string     `json:"status"`
	CreatedBy        string     `json:"createdBy"`
	StartTime        time.Time  `json:"startTime"`
	EndTime          *time.Time `json:"endTime,omitempty"`
	ClosedAt         *time.Time `json:"closedAt,omitempty"`
	DeepLinkCode     string     `json:"-"` // Internal pairing capability; never serialize
	TelegramLink     string     `json:"telegramLink,omitempty"`
	ParticipantCount *int       `json:"participantCount,omitempty"` // populated for custom_list
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
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

type SessionParticipant struct {
	SessionID string `json:"sessionId"`
	UserID    string `json:"userId"`
}
