package monitor

import "time"

type Status struct {
	PostgreSQL bool      `json:"postgresql"`
	Redis      bool      `json:"redis"`
	Buffer     bool      `json:"buffer"`
	BufferSize int       `json:"buffer_size"`
	LastCheck  time.Time `json:"last_check"`
}
