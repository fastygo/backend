package domain

import "time"

// Session represents a cached authentication session stored in Redis.
type Session struct {
	ID        string            `json:"id"`
	UserID    string            `json:"user_id"`
	ExpiresAt time.Time         `json:"expires_at"`
	CreatedAt time.Time         `json:"created_at"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

func (s *Session) IsExpired(reference time.Time) bool {
	if s == nil {
		return true
	}
	if reference.IsZero() {
		reference = time.Now()
	}
	return !s.ExpiresAt.After(reference)
}
