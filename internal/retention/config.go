package retention

import (
	"fmt"
	"time"
)

type Config struct {
	// Maximum number of backups to keep (0 = unlimited)
	MaxCount int

	// Maximum age of backups to keep (0 = unlimited)
	MaxAge time.Duration

	// Number of daily backups to keep (0 = disable)
	KeepDaily int

	// Number of weekly backups to keep (0 = disable)
	KeepWeekly int

	// Number of monthly backups to keep (0 = disable)
	KeepMonthly int

	// Dry run mode - only report what would be deleted
	DryRun bool

	// Force retention - ignore errors and continue
	Force bool
}

func (c Config) Validate() error {
	if c.MaxCount < 0 {
		return fmt.Errorf("%w: max count must be >= 0", ErrValidationFailed)
	}

	if c.MaxAge < 0 {
		return fmt.Errorf("%w: max age must be >= 0", ErrValidationFailed)
	}

	if c.KeepDaily < 0 {
		return fmt.Errorf("%w: keep daily must be >= 0", ErrValidationFailed)
	}

	if c.KeepWeekly < 0 {
		return fmt.Errorf("%w: keep weekly must be >= 0", ErrValidationFailed)
	}

	if c.KeepMonthly < 0 {
		return fmt.Errorf("%w: keep monthly must be >= 0", ErrValidationFailed)
	}

	// Validate that at least one retention policy is enabled
	if c.MaxCount == 0 && c.MaxAge == 0 && c.KeepDaily == 0 && c.KeepWeekly == 0 && c.KeepMonthly == 0 {
		return fmt.Errorf("%w: at least one retention policy must be enabled", ErrValidationFailed)
	}

	return nil
}

func (c Config) IsEmpty() bool {
	return c.MaxCount == 0 && c.MaxAge == 0 && c.KeepDaily == 0 && c.KeepWeekly == 0 && c.KeepMonthly == 0
}
