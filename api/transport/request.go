package transport

type ProfileUpdateRequest struct {
	Email   string            `json:"email"`
	Role    string            `json:"role"`
	Status  string            `json:"status"`
	Meta    map[string]string `json:"metadata"`
}

type TaskRequest struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      string            `json:"status"`
	Priority    int               `json:"priority"`
	DueDate     string            `json:"due_date"`
	Metadata    map[string]string `json:"metadata"`
}

type AuthLoginRequest struct {
	UserID string `json:"user_id"`
	TTL    int    `json:"ttl_seconds"`
}

type RefreshRequest struct {
	SessionID string `json:"session_id"`
	TTL       int    `json:"ttl_seconds"`
}

