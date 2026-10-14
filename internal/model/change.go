package model

import "time"

// Change records a detected modification to a fact across pipeline runs.
type Change struct {
	Key       string    `json:"key"`
	Period    string    `json:"period,omitempty"`
	OldValue  any       `json:"old_value"`
	NewValue  any       `json:"new_value"`
	ChangedAt time.Time `json:"changed_at"`
	SourceURL string    `json:"source_url"`
	Reason    string    `json:"reason,omitempty"`
}
