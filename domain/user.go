package domain

import "time"

// User represents an authenticated identity in the platform.
type User struct {
	ID        string            `json:"id"`
	Email     string            `json:"email,omitempty"`
	Role      string            `json:"role"`
	Status    string            `json:"status"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

func (u *User) IsActive() bool {
	return u != nil && u.Status == "active"
}
