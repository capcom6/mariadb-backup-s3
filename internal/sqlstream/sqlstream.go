// Package sqlstream implements the streamed logical backup format.
//
// A streamed logical backup carries the SQL dump of every database in a single
// stream. Each dump is preceded by a marker line naming its database, so the
// stream can be split back into one file per database:
//
//	-- mariadb-backup-s3: database=mydb
//	<dump of mydb>
//	-- mariadb-backup-s3: database=otherdb
//	<dump of otherdb>
//
// The whole stream is compressed as one gzip member, so the resulting artifact
// is a single file.
package sqlstream

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	// ArtifactExtension is the extension of the artifact produced by a
	// streamed logical backup, without the leading dot.
	ArtifactExtension = "sql.gz"

	// MarkerPrefix starts the marker line that precedes the dump of a database.
	MarkerPrefix = "-- mariadb-backup-s3: database="

	// dumpFileSuffix is the extension of the per-database files written by
	// SplitToDir.
	dumpFileSuffix = ".sql"
)

// WriteMarker writes the marker line for the given database to w.
func WriteMarker(w io.Writer, db string) error {
	if err := ValidateDatabaseName(db); err != nil {
		return err
	}

	if _, err := io.WriteString(w, MarkerPrefix+db+"\n"); err != nil {
		return fmt.Errorf("failed to write database marker: %w", err)
	}

	return nil
}

// ParseMarker returns the database name when line is a marker line.
func ParseMarker(line string) (string, bool) {
	if !strings.HasPrefix(line, MarkerPrefix) {
		return "", false
	}

	name := strings.TrimRight(strings.TrimPrefix(line, MarkerPrefix), "\r\n")

	return name, true
}

// ValidateDatabaseName reports whether the database name can be used to build
// a per-database file name.
func ValidateDatabaseName(db string) error {
	switch {
	case db == "":
		return fmt.Errorf("%w: name is empty", ErrInvalidDatabaseName)
	case db == "." || db == "..":
		return fmt.Errorf("%w: %q is a reserved name", ErrInvalidDatabaseName, db)
	case strings.ContainsAny(db, `/\`):
		return fmt.Errorf("%w: %q contains a path separator", ErrInvalidDatabaseName, db)
	case strings.ContainsFunc(db, isControl):
		return fmt.Errorf("%w: %q contains a control character", ErrInvalidDatabaseName, db)
	}

	return nil
}

// SplitToDir reads a logical backup stream and writes the dump of every
// database into its own <database>.sql file inside dir.
func SplitToDir(r io.Reader, dir string) error {
	s := &splitter{
		dir:       dir,
		current:   nil,
		seen:      make(map[string]struct{}),
		sawMarker: false,
	}
	defer func() {
		_ = s.close()
	}()

	reader := bufio.NewReader(r)
	for {
		line, readErr := reader.ReadString('\n')

		if len(line) > 0 {
			if err := s.writeLine(line); err != nil {
				return err
			}
		}

		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}

			return fmt.Errorf("failed to read stream: %w", readErr)
		}
	}

	if err := s.close(); err != nil {
		return err
	}

	if !s.sawMarker {
		return fmt.Errorf("%w", ErrNoMarker)
	}

	return nil
}

// splitter writes the lines of a logical backup stream to per-database files.
type splitter struct {
	dir       string
	current   *os.File
	seen      map[string]struct{}
	sawMarker bool
}

// writeLine consumes a single line of the stream.
func (s *splitter) writeLine(line string) error {
	db, ok := ParseMarker(line)
	if !ok {
		if !s.sawMarker {
			return fmt.Errorf("%w", ErrUnexpectedContent)
		}

		if _, err := s.current.WriteString(line); err != nil {
			return fmt.Errorf("failed to write dump file: %w", err)
		}

		return nil
	}

	if err := s.close(); err != nil {
		return err
	}

	if err := ValidateDatabaseName(db); err != nil {
		return err
	}

	if _, exists := s.seen[db]; exists {
		return fmt.Errorf("%w: %q", ErrDuplicateDatabase, db)
	}
	s.seen[db] = struct{}{}

	file, err := os.Create(filepath.Join(s.dir, db+dumpFileSuffix))
	if err != nil {
		return fmt.Errorf("failed to create dump file: %w", err)
	}
	s.current = file
	s.sawMarker = true

	return nil
}

// close closes the current dump file, if any.
func (s *splitter) close() error {
	if s.current == nil {
		return nil
	}

	err := s.current.Close()
	s.current = nil
	if err != nil {
		return fmt.Errorf("failed to close dump file: %w", err)
	}

	return nil
}

func isControl(r rune) bool {
	return r < 0x20 || r == 0x7f
}
