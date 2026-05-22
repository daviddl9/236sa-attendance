package models

import "time"

// MergeMode controls how an admin-confirmed import treats existing user
// records. Set per commit (never per row).
type MergeMode string

const (
	// MergeFillGaps only writes to fields on existing users that are
	// currently empty/NULL. Pre-existing values are preserved.
	MergeFillGaps MergeMode = "fill_gaps"
	// MergeOverride replaces all non-password fields on existing users
	// with the values from the parsed document. NRIC Last 5 / password
	// is never written on updates, regardless of mode.
	MergeOverride MergeMode = "override"
)

// ImportStatus is the lifecycle state of a single document-import job.
type ImportStatus string

const (
	ImportStatusPending   ImportStatus = "pending"
	ImportStatusPreviewed ImportStatus = "previewed"
	ImportStatusCommitted ImportStatus = "committed"
	ImportStatusFailed    ImportStatus = "failed"
)

// ImportAction is what happened to a single parsed row at commit time.
type ImportAction string

const (
	ImportActionCreated ImportAction = "created"
	ImportActionUpdated ImportAction = "updated"
	ImportActionSkipped ImportAction = "skipped"
	ImportActionFailed  ImportAction = "failed"
)

// RowCounts is the JSON summary stored on the import row.
type RowCounts struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
	Failed  int `json:"failed"`
}

// PersonnelImport is one document-import job initiated by an admin.
// Maps to the personnel_imports table.
type PersonnelImport struct {
	ID               string       `json:"id"`
	AdminUserID      string       `json:"adminUserId"`
	DocumentFilename string       `json:"documentFilename"`
	MergeMode        *MergeMode   `json:"mergeMode,omitempty"`
	RowCounts        RowCounts    `json:"rowCounts"`
	Status           ImportStatus `json:"status"`
	ErrorMessage     *string      `json:"errorMessage,omitempty"`
	StartedAt        time.Time    `json:"startedAt"`
	FinishedAt       *time.Time   `json:"finishedAt,omitempty"`
}

// PersonnelImportChange is a single per-user change recorded during
// commit. One row per changed field for updated users; one row total
// for created/skipped/failed.
type PersonnelImportChange struct {
	ID          string       `json:"id"`
	ImportID    string       `json:"importId"`
	UserID      *string      `json:"userId,omitempty"`
	Action      ImportAction `json:"action"`
	FieldName   *string      `json:"fieldName,omitempty"`
	BeforeValue *string      `json:"beforeValue,omitempty"`
	AfterValue  *string      `json:"afterValue,omitempty"`
	Reason      *string      `json:"reason,omitempty"`
	CreatedAt   time.Time    `json:"createdAt"`
}
