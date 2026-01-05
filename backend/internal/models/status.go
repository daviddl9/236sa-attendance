package models

import "time"

// Status types
const (
	StatusTypeStayOut = "stay_out"
	StatusTypeOffPass = "off_pass"
	StatusTypeMC      = "mc"
	StatusTypeCourse  = "course"
	StatusTypeOther   = "other"
)

// ValidStatusTypes for validation
var ValidStatusTypes = []string{
	StatusTypeStayOut,
	StatusTypeOffPass,
	StatusTypeMC,
	StatusTypeCourse,
	StatusTypeOther,
}

// StatusTypeDisplayNames maps internal types to display names
var StatusTypeDisplayNames = map[string]string{
	StatusTypeStayOut: "Stay Out",
	StatusTypeOffPass: "Off Pass",
	StatusTypeMC:      "MC",
	StatusTypeCourse:  "Course",
	StatusTypeOther:   "Other",
}

// IsValidStatusType checks if status type is valid
func IsValidStatusType(statusType string) bool {
	for _, t := range ValidStatusTypes {
		if t == statusType {
			return true
		}
	}
	return false
}

type UserStatus struct {
	ID             string           `json:"id"`
	UserID         string           `json:"userId"`
	StatusType     string           `json:"statusType"`
	DailyAfterTime *string          `json:"dailyAfterTime,omitempty"` // for stay_out "18:30"
	StartDate      *string          `json:"startDate,omitempty"`      // for mc "2026-01-15"
	EndDate        *string          `json:"endDate,omitempty"`        // for mc "2026-01-20"
	Notes          *string          `json:"notes,omitempty"`
	Dates          []UserStatusDate `json:"dates,omitempty"` // for off_pass, course, other
	CreatedBy      string           `json:"createdBy"`
	CreatedAt      time.Time        `json:"createdAt"`
	UpdatedAt      time.Time        `json:"updatedAt"`
}

type UserStatusDate struct {
	ID        string `json:"id"`
	StatusID  string `json:"statusId"`
	StartDate string `json:"startDate"` // "2026-01-15"
	StartTime string `json:"startTime"` // "09:00"
	EndDate   string `json:"endDate"`   // "2026-01-16"
	EndTime   string `json:"endTime"`   // "18:00"
}

// UserStatusWithUser includes user info for list display
type UserStatusWithUser struct {
	UserStatus
	UserFullName *string `json:"userFullName,omitempty"`
	UserRank     *string `json:"userRank,omitempty"`
	UserBattery  *string `json:"userBattery,omitempty"`
}

// StatusInfo is a simplified view for display in attendance lists
type StatusInfo struct {
	StatusType  string `json:"statusType"`
	DisplayName string `json:"displayName"`
}

// GetDisplayName returns the human-readable status type name
func (s *UserStatus) GetDisplayName() string {
	if name, ok := StatusTypeDisplayNames[s.StatusType]; ok {
		return name
	}
	return s.StatusType
}
