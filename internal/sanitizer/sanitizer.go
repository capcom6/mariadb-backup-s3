package sanitizer

import (
	"fmt"
	"regexp"
	"strings"
)

func SanitizeOptions(options string) ([]string, error) {
	// Check for dangerous characters that could lead to command injection
	dangerousPattern := regexp.MustCompile(`[;&|$\n\r]`)
	if dangerousPattern.MatchString(options) {
		return nil, fmt.Errorf("backup options contain potentially dangerous characters")
	}

	// Split options into individual arguments
	args := strings.Fields(options)

	// Validate each argument to ensure it's a safe mariabackup option
	safeArgs := make([]string, 0, len(args))
	for _, arg := range args {
		if err := validateOption(arg); err != nil {
			return nil, fmt.Errorf("invalid backup option '%s': %w", arg, err)
		}
		safeArgs = append(safeArgs, arg)
	}

	return safeArgs, nil
}

// validateOption validates a single backup option to prevent command injection
func validateOption(option string) error {
	// Basic pattern for safe options:
	// --flag=value or --flag or --flag value
	// Only allows letters, numbers, hyphens, underscores, equals, dots, and spaces
	validPattern := regexp.MustCompile(`^--([a-zA-Z0-9_-]+)(?:=([a-zA-Z0-9_./-]+))?$`)

	if !validPattern.MatchString(option) {
		return fmt.Errorf("option contains invalid characters or format")
	}

	return nil
}
