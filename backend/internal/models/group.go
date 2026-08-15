package models

import "time"

// ParticipantGroup is a named, reusable participant list decoupled from any
// single session, so a recurring group (e.g. the advance party) is created once
// and reused for every callup.
type ParticipantGroup struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	CreatedBy  string    `json:"createdBy"`
	MemberCount *int      `json:"memberCount,omitempty"` // populated on list/get
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// ParticipantGroupMember links a group to a roster user.
type ParticipantGroupMember struct {
	GroupID string `json:"groupId"`
	UserID  string `json:"userId"`
}
