package webhooks

import "time"

type Config struct {
	DefaultURL     string
	DefaultTimeout time.Duration
}
