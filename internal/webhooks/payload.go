package webhooks

import (
	"encoding/json"
	"fmt"
	"time"
)

type Duration time.Duration //nolint:recvcheck // For marshaling

func (d Duration) String() string {
	return time.Duration(d).String()
}

func (d Duration) MarshalJSON() ([]byte, error) {
	s, err := json.Marshal(d.String())
	if err != nil {
		return nil, fmt.Errorf("marshal duration: %w", err)
	}
	return s, nil
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("unmarshal duration: %w", err)
	}
	dur, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("parse duration string %q: %w", s, err)
	}
	*d = Duration(dur)
	return nil
}

type Payload struct {
	Event     string    `json:"event"`
	JobName   string    `json:"job_name"`
	Status    string    `json:"status"`
	StartedAt time.Time `json:"started_at,omitzero"`
	Duration  Duration  `json:"duration,omitempty"`
	Error     string    `json:"error,omitempty"`
}
