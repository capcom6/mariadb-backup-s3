package method

// Config holds the settings shared by all backup method implementations.
type Config struct {
	Host          string
	Port          int
	User          string
	Password      string
	BackupOptions string
	BackupBinary  string
	ClientBinary  string

	// Stream makes the method emit its output as a stream instead of writing
	// files into a temp directory. Only logical backups support it.
	Stream bool
}
