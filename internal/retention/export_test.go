package retention

import (
	"context"

	"github.com/capcom6/mariadb-backup-s3/internal/registry"
)

func (o *Operation) ApplyMaxCountPolicy(
	backups []registry.BackupEntry,
) ([]registry.BackupEntry, []registry.BackupEntry) {
	return o.applyMaxCountPolicy(context.Background(), backups)
}

func (o *Operation) ApplyMaxAgePolicy(backups []registry.BackupEntry) ([]registry.BackupEntry, []registry.BackupEntry) {
	return o.applyMaxAgePolicy(context.Background(), backups)
}

func (o *Operation) SelectPeriodBackups(
	backups []registry.BackupEntry,
	groupFn func(registry.BackupEntry) string,
	count int,
) []registry.BackupEntry {
	return o.selectPeriodBackups(context.Background(), backups, groupFn, count)
}

func (o *Operation) ApplyPeriodicPolicies(
	backups []registry.BackupEntry,
) ([]registry.BackupEntry, []registry.BackupEntry) {
	return o.applyPeriodicPolicies(context.Background(), backups)
}

func (o *Operation) FilterBackups(reg *registry.Registry) ([]registry.BackupEntry, error) {
	return o.filterBackups(context.Background(), reg)
}

func (o *Operation) DeleteBackup(backup registry.BackupEntry) error {
	return o.deleteBackup(context.Background(), backup)
}

func (o *Operation) ExecuteRetentionActions(backupsToRemove []registry.BackupEntry) ([]registry.BackupEntry, error) {
	return o.executeRetentionActions(context.Background(), backupsToRemove)
}

func (o *Operation) ApplyRetentionPolicies(backups []registry.BackupEntry) ([]registry.BackupEntry, error) {
	return o.applyRetentionPolicies(context.Background(), backups)
}

func (o *Operation) LoadRegistry() (*registry.Registry, error) {
	return o.loadRegistry(context.Background())
}

func (o *Operation) UpdateRegistry(backupsToRemove []registry.BackupEntry) error {
	return o.updateRegistry(context.Background(), backupsToRemove)
}
