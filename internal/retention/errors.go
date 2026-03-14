package retention

import "errors"

var (
	// ErrValidationFailed indicates configuration validation failed.
	ErrValidationFailed = errors.New("retention validation failed")

	// ErrRegistryLoadFailed indicates failure to load backup registry.
	ErrRegistryLoadFailed = errors.New("failed to load backup registry")

	// ErrRegistryUpdateFailed indicates failure to update backup registry.
	ErrRegistryUpdateFailed = errors.New("failed to update backup registry")

	// ErrStorageOperationFailed indicates failure in storage operation.
	ErrStorageOperationFailed = errors.New("storage operation failed")

	// ErrRetentionPolicyConflict indicates conflicting retention policies.
	ErrRetentionPolicyConflict = errors.New("retention policy conflict")

	// ErrBackupNotFound indicates backup not found in registry.
	ErrBackupNotFound = errors.New("backup not found")

	// ErrBackupDeleteFailed indicates failure to delete backup file.
	ErrBackupDeleteFailed = errors.New("failed to delete backup file")
)
