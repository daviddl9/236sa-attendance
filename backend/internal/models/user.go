package models

import (
	"strings"
	"time"
)

// Singapore military ranks ordered from lowest to highest
const (
	RankREC = "REC" // Recruit
	RankPTE = "PTE" // Private
	RankLCP = "LCP" // Lance Corporal
	RankCPL = "CPL" // Corporal
	RankCFC = "CFC" // Corporal First Class
	Rank3SG = "3SG" // Third Sergeant
	Rank2SG = "2SG" // Second Sergeant
	Rank1SG = "1SG" // First Sergeant
	RankSSG = "SSG" // Staff Sergeant
	RankMSG = "MSG" // Master Sergeant
	Rank3WO = "3WO" // Third Warrant Officer
	Rank2WO = "2WO" // Second Warrant Officer
	Rank1WO = "1WO" // First Warrant Officer
	RankMWO = "MWO" // Master Warrant Officer
	RankSWO = "SWO" // Senior Warrant Officer
	Rank2LT = "2LT" // Second Lieutenant
	RankLTA = "LTA" // Lieutenant
	RankCPT = "CPT" // Captain
	RankMAJ = "MAJ" // Major
	RankLTC = "LTC" // Lieutenant Colonel
)

// Batteries
const (
	BatteryHQ    = "HQ"
	BatteryAlpha = "Alpha"
	BatteryBravo = "Bravo"
)

// Valid ranks list for validation
var ValidRanks = []string{
	RankREC, RankPTE, RankLCP, RankCPL, RankCFC,
	Rank3SG, Rank2SG, Rank1SG, RankSSG, RankMSG,
	Rank3WO, Rank2WO, Rank1WO, RankMWO, RankSWO,
	Rank2LT, RankLTA, RankCPT, RankMAJ, RankLTC,
}

// Rank order map (lower number = lower rank)
var RankOrder = map[string]int{
	RankREC: 0, RankPTE: 1, RankLCP: 2, RankCPL: 3, RankCFC: 4,
	Rank3SG: 5, Rank2SG: 6, Rank1SG: 7, RankSSG: 8, RankMSG: 9,
	Rank3WO: 10, Rank2WO: 11, Rank1WO: 12, RankMWO: 13, RankSWO: 14,
	Rank2LT: 15, RankLTA: 16, RankCPT: 17, RankMAJ: 18, RankLTC: 19,
}

// AccessTier represents the user's access level.
type AccessTier int

const (
	TierEnlisted      AccessTier = 1 // REC–CFC
	TierBatteryNCO    AccessTier = 2 // 3SG–1SG (or tier_override=2)
	TierUnitCommander AccessTier = 3 // SSG+ (or tier_override=3)
	TierSuperadmin    AccessTier = 4 // is_superadmin=true or CPT+
)

type User struct {
	ID                     string            `json:"id"`
	FullName               *string           `json:"fullName,omitempty"`
	Rank                   *string           `json:"rank,omitempty"`
	Battery                *string           `json:"battery,omitempty"`
	NRICLast5              *string           `json:"nricLast5,omitempty"`
	DOB                    *string           `json:"dob,omitempty"` // DDMMYY format
	Extras                 map[string]string `json:"extras"`
	TierOverride           *int16            `json:"tierOverride,omitempty"` // 2 or 3; nil = rank-derived
	Verified               bool              `json:"verified"`
	PasswordChangeRequired bool              `json:"passwordChangeRequired"`
	IsSuperadmin           bool              `json:"isSuperadmin"`
	CreatedAt              time.Time         `json:"createdAt"`
	UpdatedAt              time.Time         `json:"updatedAt"`
}

// GetTier returns the effective access tier for the user.
func (u *User) GetTier() AccessTier {
	if u.IsSuperadmin {
		return TierSuperadmin
	}
	derived := u.rankDerivedTier()
	if u.TierOverride != nil && AccessTier(*u.TierOverride) > derived {
		return AccessTier(*u.TierOverride)
	}
	return derived
}

func (u *User) rankDerivedTier() AccessTier {
	if u.Rank == nil {
		return TierEnlisted
	}
	order, exists := RankOrder[strings.ToUpper(*u.Rank)]
	if !exists {
		return TierEnlisted
	}
	if order >= RankOrder[RankCPT] { // CPT+ → superadmin
		return TierSuperadmin
	}
	if order >= RankOrder[RankSSG] { // SSG+ → unit commander
		return TierUnitCommander
	}
	if order >= RankOrder[Rank3SG] { // 3SG–1SG → battery NCO
		return TierBatteryNCO
	}
	return TierEnlisted
}

// IsSuperadminByRank returns true when the rank alone warrants superadmin (CPT+).
// Used to auto-set is_superadmin on create/update.
func IsSuperadminByRank(rank string) bool {
	order, exists := RankOrder[strings.ToUpper(rank)]
	return exists && order >= RankOrder[RankCPT]
}

// IsCommander returns true if user can access commander-level features (tier >= 2).
func (u *User) IsCommander() bool {
	return u.GetTier() >= TierBatteryNCO
}

// IsUnitCommander returns true if user can access unit-level features (tier >= 3).
func (u *User) IsUnitCommander() bool {
	return u.GetTier() >= TierUnitCommander
}

// GetRole returns a string role label for the user.
func (u *User) GetRole() string {
	switch u.GetTier() {
	case TierSuperadmin:
		return "superadmin"
	case TierUnitCommander:
		return "unit_commander"
	case TierBatteryNCO:
		return "battery_nco"
	default:
		return "user"
	}
}

// GetDisplayName returns full_name if available
func (u *User) GetDisplayName() string {
	if u.FullName != nil && *u.FullName != "" {
		return *u.FullName
	}
	return "Unknown User"
}

type Session struct {
	ID        string    `json:"id"`
	ExpiresAt time.Time `json:"expiresAt"`
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	IPAddress *string   `json:"ipAddress,omitempty"`
	UserAgent *string   `json:"userAgent,omitempty"`
	UserID    string    `json:"userId"`
}

type Account struct {
	ID                    string     `json:"id"`
	AccountID             string     `json:"accountId"`
	ProviderID            string     `json:"providerId"`
	UserID                string     `json:"userId"`
	AccessToken           *string    `json:"accessToken,omitempty"`
	RefreshToken          *string    `json:"refreshToken,omitempty"`
	IDToken               *string    `json:"idToken,omitempty"`
	AccessTokenExpiresAt  *time.Time `json:"accessTokenExpiresAt,omitempty"`
	RefreshTokenExpiresAt *time.Time `json:"refreshTokenExpiresAt,omitempty"`
	Scope                 *string    `json:"scope,omitempty"`
	Password              *string    `json:"-"` // Never serialize password
	CreatedAt             time.Time  `json:"createdAt"`
	UpdatedAt             time.Time  `json:"updatedAt"`
}

type Verification struct {
	ID         string    `json:"id"`
	Identifier string    `json:"identifier"`
	Value      string    `json:"value"`
	ExpiresAt  time.Time `json:"expiresAt"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type Subscription struct {
	ID                          string     `json:"id"`
	CreatedAt                   time.Time  `json:"createdAt"`
	ModifiedAt                  *time.Time `json:"modifiedAt,omitempty"`
	Amount                      int        `json:"amount"`
	Currency                    string     `json:"currency"`
	RecurringInterval           string     `json:"recurringInterval"`
	Status                      string     `json:"status"`
	CurrentPeriodStart          time.Time  `json:"currentPeriodStart"`
	CurrentPeriodEnd            time.Time  `json:"currentPeriodEnd"`
	CancelAtPeriodEnd           bool       `json:"cancelAtPeriodEnd"`
	CanceledAt                  *time.Time `json:"canceledAt,omitempty"`
	StartedAt                   time.Time  `json:"startedAt"`
	EndsAt                      *time.Time `json:"endsAt,omitempty"`
	EndedAt                     *time.Time `json:"endedAt,omitempty"`
	CustomerID                  string     `json:"customerId"`
	ProductID                   string     `json:"productId"`
	DiscountID                  *string    `json:"discountId,omitempty"`
	CheckoutID                  string     `json:"checkoutId"`
	CustomerCancellationReason  *string    `json:"customerCancellationReason,omitempty"`
	CustomerCancellationComment *string    `json:"customerCancellationComment,omitempty"`
	Metadata                    *string    `json:"metadata,omitempty"`        // JSON string
	CustomFieldData             *string    `json:"customFieldData,omitempty"` // JSON string
	UserID                      *string    `json:"userId,omitempty"`
}
