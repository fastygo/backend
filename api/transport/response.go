package transport

import "encoding/json"

// Envelope is the standard API response wrapper used for both success and error payloads.
type Envelope struct {
	Status string      `json:"status"`
	Code   string      `json:"code,omitempty"`
	Data   interface{} `json:"data,omitempty"`
	Error  interface{} `json:"error,omitempty"`
	Meta   interface{} `json:"meta,omitempty"`
}

// NewSuccess returns a success envelope.
func NewSuccess(data interface{}, meta interface{}) Envelope {
	return Envelope{
		Status: "success",
		Data:   data,
		Meta:   meta,
	}
}

// NewError returns an error envelope with optional metadata.
func NewError(code string, err interface{}, meta interface{}) Envelope {
	return Envelope{
		Status: "error",
		Code:   code,
		Error:  err,
		Meta:   meta,
	}
}

// String returns the JSON representation (best-effort) for logging purposes.
func (e Envelope) String() string {
	out, err := json.Marshal(e)
	if err != nil {
		return "{}"
	}
	return string(out)
}
