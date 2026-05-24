package retention

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/capcom6/mariadb-backup-s3/internal/logging"
	"github.com/capcom6/mariadb-backup-s3/internal/registry"
	"github.com/capcom6/mariadb-backup-s3/internal/storage"
)

const (
	logFieldSizeBytes   = "size_bytes"
	logFieldCreatedAt   = "created_at"
	logFieldFilename    = "filename"
	logFieldRemoveCount = "remove_count"
	logFieldKeepCount   = "keep_count"
	logFieldDuration    = "duration"
	logFieldMaxCount    = "max_count"
	logFieldMaxAge      = "max_age"
	logFieldKeepDaily   = "keep_daily"
	logFieldKeepWeekly  = "keep_weekly"
	logFieldKeepMonthly = "keep_monthly"
	logFieldDryRun      = "dry_run"
	logFieldForce       = "force"
)

type Operation struct {
	config Config

	// Registry service for backup metadata operations
	registrySvc *registry.Service

	// Storage backend for file operations
	storage storage.Backend

	// Logger for structured logging
	logger logging.Logger
}

func NewOperation(
	config Config,
	registrySvc *registry.Service,
	storage storage.Backend,
	logger logging.Logger,
) *Operation {
	return &Operation{
		config: config,

		registrySvc: registrySvc,

		storage: storage,

		logger: logger,
	}
}

// Run executes the retention operation with staged execution.
func (o *Operation) Run(ctx context.Context) error {
	ctx = logging.WithComponent(ctx, "retention")

	o.logger.Info(ctx, "Starting retention operation", logging.Fields{
		logFieldMaxCount:    o.config.MaxCount,
		logFieldMaxAge:      o.config.MaxAge.String(),
		logFieldKeepDaily:   o.config.KeepDaily,
		logFieldKeepWeekly:  o.config.KeepWeekly,
		logFieldKeepMonthly: o.config.KeepMonthly,
		logFieldDryRun:      o.config.DryRun,
		logFieldForce:       o.config.Force,
	})

	if o.config.IsEmpty() {
		o.logger.Info(ctx, "No retention policies enabled, skipping retention operation")
		return nil
	}

	start := time.Now()
	defer func() {
		duration := time.Since(start)
		o.logger.Info(ctx, "Retention operation completed", logging.Fields{
			logFieldDuration: duration.String(),
		})
	}()

	var err error

	// Stage 1: Load and validate registry
	var reg *registry.Registry
	if reg, err = o.loadRegistry(ctx); err != nil {
		return fmt.Errorf("failed to load registry: %w", err)
	}

	// Stage 2: Filter and categorize backups
	var readyBackups []registry.BackupEntry
	if readyBackups, err = o.filterBackups(ctx, reg); err != nil {
		return fmt.Errorf("failed to filter backups: %w", err)
	}

	// Stage 3: Apply retention policies
	var toRemove []registry.BackupEntry
	if toRemove, err = o.applyRetentionPolicies(ctx, readyBackups); err != nil {
		return fmt.Errorf("failed to apply retention policies: %w", err)
	}

	// Stage 4: Execute retention actions
	var successfullyDeleted []registry.BackupEntry
	if successfullyDeleted, err = o.executeRetentionActions(ctx, toRemove); err != nil {
		return fmt.Errorf("failed to execute retention actions: %w", err)
	}

	// Stage 5: Update registry
	if updErr := o.updateRegistry(ctx, successfullyDeleted); updErr != nil {
		return fmt.Errorf("failed to update registry: %w", updErr)
	}

	return nil
}

// loadRegistry loads the backup registry from storage.
func (o *Operation) loadRegistry(ctx context.Context) (*registry.Registry, error) {
	o.logger.Info(ctx, "Stage 1: Load registry")
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		o.logger.Info(ctx, "Stage 1 completed", logging.Fields{
			logFieldDuration: duration.String(),
		})
	}()

	registry, err := o.registrySvc.Load(ctx)
	if err != nil {
		o.logger.Error(ctx, "Failed to load backup registry", err)
		return nil, fmt.Errorf("%w: %w", ErrRegistryLoadFailed, err)
	}

	// Log additional context about loaded registry
	o.logger.Debug(ctx, "Registry loaded", logging.Fields{
		"backup_count":     len(registry.Backups),
		"registry_version": registry.Version,
		"registry_updated": registry.UpdatedAt.Format(time.RFC3339),
	})

	return registry, nil
}

// filterBackups categorizes backups based on retention policies.
func (o *Operation) filterBackups(ctx context.Context, reg *registry.Registry) ([]registry.BackupEntry, error) {
	o.logger.Info(ctx, "Stage 2: Filter backups")
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		o.logger.Info(ctx, "Stage 2 completed", logging.Fields{
			logFieldDuration: duration.String(),
		})
	}()

	// Filter only ready backups
	var readyBackups []registry.BackupEntry
	for _, backup := range reg.Backups {
		if backup.Status == registry.StatusReady {
			readyBackups = append(readyBackups, backup)
		}
	}

	o.logger.Debug(ctx, "Filtered ready backups", logging.Fields{
		"total_backups": len(reg.Backups),
		"ready_backups": len(readyBackups),
	})

	return readyBackups, nil
}

// applyRetentionPolicies determines which backups to keep or delete.
func (o *Operation) applyRetentionPolicies(
	ctx context.Context,
	backups []registry.BackupEntry,
) ([]registry.BackupEntry, error) {
	o.logger.Info(ctx, "Stage 3: Apply retention policies")
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		o.logger.Info(ctx, "Stage 3 completed", logging.Fields{
			logFieldDuration: duration.String(),
		})
	}()

	// Apply all policies against the original backups slice to create union of kept items
	keepSet := make(map[string]bool) // Union of all items to keep

	// Apply MaxCount policy
	if o.config.MaxCount > 0 {
		keep, remove := o.applyMaxCountPolicy(ctx, backups)

		o.logger.Debug(ctx, "Applied MaxCount policy", logging.Fields{
			logFieldKeepCount:   len(keep),
			logFieldRemoveCount: len(remove),
			"max_count":         o.config.MaxCount,
		})

		// Add items to keep set
		for _, backup := range keep {
			keepSet[backup.ID] = true
		}
	}

	// Apply MaxAge policy
	if o.config.MaxAge > 0 {
		keep, remove := o.applyMaxAgePolicy(ctx, backups)

		o.logger.Debug(ctx, "Applied MaxAge policy", logging.Fields{
			logFieldKeepCount:   len(keep),
			logFieldRemoveCount: len(remove),
			"max_age":           o.config.MaxAge.String(),
		})

		// Add items to keep set
		for _, backup := range keep {
			keepSet[backup.ID] = true
		}
	}

	// Apply periodic policies (daily, weekly, monthly)
	if o.config.KeepDaily > 0 || o.config.KeepWeekly > 0 || o.config.KeepMonthly > 0 {
		keep, remove := o.applyPeriodicPolicies(ctx, backups)

		o.logger.Debug(ctx, "Applied periodic policies", logging.Fields{
			logFieldKeepCount:   len(keep),
			logFieldRemoveCount: len(remove),
			"keep_daily":        o.config.KeepDaily,
			"keep_weekly":       o.config.KeepWeekly,
			"keep_monthly":      o.config.KeepMonthly,
		})

		// Add items to keep set
		for _, backup := range keep {
			keepSet[backup.ID] = true
		}
	}

	// Build final keep and remove lists from the union set
	var finalKeep []registry.BackupEntry
	var finalRemove []registry.BackupEntry

	for _, backup := range backups {
		if keepSet[backup.ID] {
			finalKeep = append(finalKeep, backup)
		} else {
			finalRemove = append(finalRemove, backup)
		}
	}

	o.logger.Info(ctx, "Retention policy application completed", logging.Fields{
		"total_backups":     len(finalKeep) + len(finalRemove),
		logFieldKeepCount:   len(finalKeep),
		logFieldRemoveCount: len(finalRemove),
	})

	return finalRemove, nil
}

// executeRetentionActions performs the actual deletion operations.
func (o *Operation) executeRetentionActions(
	ctx context.Context,
	backupsToRemove []registry.BackupEntry,
) ([]registry.BackupEntry, error) {
	o.logger.Info(ctx, "Stage 4: Execute retention actions")
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		o.logger.Info(ctx, "Stage 4 completed", logging.Fields{
			logFieldDuration: duration.String(),
		})
	}()

	// Log summary of retention decisions
	o.logger.Info(ctx, "Retention summary", logging.Fields{
		logFieldRemoveCount: len(backupsToRemove),
		"dry_run":           o.config.DryRun,
	})

	// Log details for each backup being removed
	for _, backup := range backupsToRemove {
		if o.config.DryRun {
			o.logger.Info(ctx, "Dry run: would remove backup", logging.Fields{
				logFieldFilename:  backup.Filename,
				logFieldCreatedAt: backup.CreatedAt.Format(time.RFC3339),
				logFieldSizeBytes: backup.SizeBytes,
				"status":          backup.Status,
			})
		} else {
			o.logger.Info(ctx, "Removing backup", logging.Fields{
				logFieldFilename:  backup.Filename,
				logFieldCreatedAt: backup.CreatedAt.Format(time.RFC3339),
				logFieldSizeBytes: backup.SizeBytes,
				"status":          backup.Status,
			})
		}
	}

	// Delete backup files
	var successfullyDeleted []registry.BackupEntry
	for _, backup := range backupsToRemove {
		if err := o.deleteBackup(ctx, backup); err != nil {
			if !o.config.Force {
				return nil, err
			}
			// In force mode, log error but continue
			o.logger.Error(ctx, "Failed to delete backup but continuing due to force mode", err, logging.Fields{
				logFieldFilename: backup.Filename,
			})
		} else {
			// Only add to successfullyDeleted if deleteBackup returned nil
			successfullyDeleted = append(successfullyDeleted, backup)
		}
	}

	if o.config.DryRun {
		return nil, nil
	}

	return successfullyDeleted, nil
}

// updateRegistry saves the updated registry after retention operations.
func (o *Operation) updateRegistry(ctx context.Context, backupsToRemove []registry.BackupEntry) error {
	o.logger.Info(ctx, "Stage 5: Update registry")
	start := time.Now()
	defer func() {
		duration := time.Since(start)
		o.logger.Info(ctx, "Stage 5 completed", logging.Fields{
			logFieldDuration: duration.String(),
		})
	}()

	// Mark successfully deleted backups as deleted in the registry
	var successfullyMarked []registry.BackupEntry
	for _, backup := range backupsToRemove {
		if err := o.registrySvc.MarkDeleted(ctx, backup.ID); err != nil {
			o.logger.Error(ctx, "Failed to mark backup as deleted", err, logging.Fields{
				"backup_id":      backup.ID,
				logFieldFilename: backup.Filename,
				"error":          err.Error(),
			})

			if !o.config.Force {
				return fmt.Errorf(
					"%w: failed to mark backup %s as deleted: %w",
					ErrRegistryUpdateFailed,
					backup.Filename,
					err,
				)
			}

			o.logger.Warn(ctx, "Continuing despite registry update failure due to force mode", logging.Fields{
				"backup_id":      backup.ID,
				logFieldFilename: backup.Filename,
			})
		} else {
			// Only add to successfullyMarked if MarkDeleted returned nil
			successfullyMarked = append(successfullyMarked, backup)
		}
	}

	o.logger.Debug(ctx, "Registry updated", logging.Fields{
		"removed_count": len(successfullyMarked),
	})

	return nil
}

// applyMaxCountPolicy applies the maximum count retention policy.
func (o *Operation) applyMaxCountPolicy(
	_ context.Context,
	backups []registry.BackupEntry,
) ([]registry.BackupEntry, []registry.BackupEntry) {
	if o.config.MaxCount == 0 {
		return backups, nil // No limit
	}

	if len(backups) <= o.config.MaxCount {
		return backups, nil // Under limit
	}

	// Sort by creation time (newest first)
	sorted := make([]registry.BackupEntry, len(backups))
	copy(sorted, backups)

	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt.After(sorted[j].CreatedAt)
	})

	// Keep first MaxCount backups
	keep := sorted[:o.config.MaxCount]
	remove := sorted[o.config.MaxCount:]

	return keep, remove
}

// applyMaxAgePolicy applies the maximum age retention policy.
func (o *Operation) applyMaxAgePolicy(
	_ context.Context,
	backups []registry.BackupEntry,
) ([]registry.BackupEntry, []registry.BackupEntry) {
	if o.config.MaxAge == 0 {
		return backups, nil // No age limit
	}

	now := time.Now()
	cutoff := now.Add(-o.config.MaxAge)

	var keep, remove []registry.BackupEntry

	for _, backup := range backups {
		if backup.CreatedAt.After(cutoff) {
			keep = append(keep, backup)
		} else {
			remove = append(remove, backup)
		}
	}

	return keep, remove
}

// applyPeriodicPolicies applies daily, weekly, and monthly retention policies.
func (o *Operation) applyPeriodicPolicies(
	ctx context.Context,
	backups []registry.BackupEntry,
) ([]registry.BackupEntry, []registry.BackupEntry) {
	var keep, remove []registry.BackupEntry

	// Create a map to track which backups to keep
	keepMap := make(map[string]bool)

	// Apply daily policy
	if o.config.KeepDaily > 0 {
		dailyKeep := o.selectPeriodBackups(
			ctx,
			backups,
			func(backup registry.BackupEntry) string { return backup.CreatedAt.Format("2006-01-02") },
			o.config.KeepDaily,
		)

		for _, backup := range dailyKeep {
			keepMap[backup.ID] = true
		}
	}

	// Apply weekly policy
	if o.config.KeepWeekly > 0 {
		weeklyKeep := o.selectPeriodBackups(ctx, backups, func(backup registry.BackupEntry) string {
			year, week := backup.CreatedAt.ISOWeek()
			return fmt.Sprintf("%d-W%02d", year, week)
		}, o.config.KeepWeekly)

		for _, backup := range weeklyKeep {
			keepMap[backup.ID] = true
		}
	}

	// Apply monthly policy
	if o.config.KeepMonthly > 0 {
		monthlyKeep := o.selectPeriodBackups(
			ctx,
			backups,
			func(backup registry.BackupEntry) string { return backup.CreatedAt.Format("2006-01") },
			o.config.KeepMonthly,
		)
		for _, backup := range monthlyKeep {
			keepMap[backup.ID] = true
		}
	}

	// Separate keep and remove based on the keepMap
	for _, backup := range backups {
		if keepMap[backup.ID] {
			keep = append(keep, backup)
		} else {
			remove = append(remove, backup)
		}
	}

	return keep, remove
}

// selectPeriodBackups selects the most recent backups for a given period.
func (o *Operation) selectPeriodBackups(
	_ context.Context,
	backups []registry.BackupEntry,
	groupFn func(backup registry.BackupEntry) string,
	count int,
) []registry.BackupEntry {
	// Group backups by day
	groups := make(map[string][]registry.BackupEntry)

	for _, backup := range backups {
		groupKey := groupFn(backup)
		groups[groupKey] = append(groups[groupKey], backup)
	}

	// For each day, select the most recent backup
	var selected []registry.BackupEntry
	for _, group := range groups {
		// Sort by creation time (newest first)
		sort.Slice(group, func(i, j int) bool {
			return group[i].CreatedAt.After(group[j].CreatedAt)
		})

		// Take the most recent backup for this day
		if len(group) > 0 {
			selected = append(selected, group[0])
		}
	}

	// Sort all selected backups by creation time (newest first)
	sort.Slice(selected, func(i, j int) bool {
		return selected[i].CreatedAt.After(selected[j].CreatedAt)
	})

	// Limit to the requested count
	if len(selected) > count {
		selected = selected[:count]
	}

	return selected
}

// deleteBackup deletes a backup file from storage.
func (o *Operation) deleteBackup(ctx context.Context, backup registry.BackupEntry) error {
	if o.config.DryRun {
		o.logger.Info(ctx, "Dry run: would delete backup", logging.Fields{
			logFieldFilename:  backup.Filename,
			logFieldCreatedAt: backup.CreatedAt.Format(time.RFC3339),
			logFieldSizeBytes: backup.SizeBytes,
		})
		return nil
	}

	o.logger.Info(ctx, "Deleting backup", logging.Fields{
		logFieldFilename:  backup.Filename,
		logFieldCreatedAt: backup.CreatedAt.Format(time.RFC3339),
		logFieldSizeBytes: backup.SizeBytes,
	})

	if err := o.storage.Delete(ctx, backup.Filename); err != nil {
		o.logger.Error(ctx, "Failed to delete backup file", err, logging.Fields{
			logFieldFilename: backup.Filename,
			"error":          err.Error(),
		})

		return fmt.Errorf("%w: failed to delete backup %s: %w", ErrBackupDeleteFailed, backup.Filename, err)
	}

	return nil
}
