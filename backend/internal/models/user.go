package models

import "time"

type User struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"emailVerified"`
	Image         *string   `json:"image,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
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
