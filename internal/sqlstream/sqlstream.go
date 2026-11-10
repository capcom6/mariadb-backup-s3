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
//
// Dump content that starts with the marker prefix (or with the escape byte) is
// prefixed with the escape byte by NewEscaper and stripped again by SplitToDir,
// so only markers written by WriteMarker parse as database markers.
package sqlstream

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
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

	// dumpFilePerm is the permission requested for per-database dump files
	// before the process umask is applied, matching [os.Create].
	dumpFilePerm = 0o666

	// escapeByte prefixes dump-content lines whose start would otherwise parse
	// as a marker line. mariadb-dump escapes NUL bytes in dump output, so
	// artifacts written before escaping was introduced never begin a line with
	// it.
	escapeByte = '\x00'

	// maxDatabaseNameBytes is the largest database name a marker can carry:
	// MariaDB identifiers are at most 64 characters of up to 4 bytes each.
	maxDatabaseNameBytes = 64 * 4

	// maxMarkerLineLen is the longest line WriteMarker can produce, including
	// a CRLF line ending. Anything longer cannot be a valid marker, so
	// bufferMarker rejects it before buffering.
	maxMarkerLineLen = len(MarkerPrefix) + maxDatabaseNameBytes + 2
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

// Escaper prefixes marker-like line starts in dump content with escapeByte so
// SplitToDir cannot mistake them for database markers. Call Flush after the
// last Write.
type Escaper struct {
	w           io.Writer
	pending     []byte // line-start bytes awaiting classification
	atLineStart bool
}

// NewEscaper returns a writer that escapes marker-like line starts for w.
// Each dump must be wrapped in its own Escaper, starting at a line start.
func NewEscaper(w io.Writer) *Escaper {
	return &Escaper{w: w, pending: nil, atLineStart: true}
}

// Write escapes marker-like line starts of p before passing everything
// through to the underlying writer.
func (e *Escaper) Write(p []byte) (int, error) {
	consumed := 0
	for len(p) > 0 {
		var (
			n   int
			err error
		)
		if e.atLineStart {
			n, err = e.writeLineStart(p)
		} else {
			n, err = e.writeThrough(p)
		}
		consumed += n
		p = p[n:]
		if err != nil {
			return consumed, err
		}
	}

	return consumed, nil
}

// Flush writes any buffered line-start bytes to the underlying writer.
func (e *Escaper) Flush() error {
	if len(e.pending) == 0 {
		return nil
	}

	_, err := e.writeAll(e.pending)
	e.pending = nil

	return err
}

// writeAll passes p to the underlying writer and wraps a write failure.
func (e *Escaper) writeAll(p []byte) (int, error) {
	n, err := e.w.Write(p)
	if err != nil {
		return n, fmt.Errorf("failed to write escaped stream: %w", err)
	}

	return n, nil
}

// writeThrough passes p to the underlying writer up to and including the next
// newline, ending the line. It returns the number of bytes of p consumed.
func (e *Escaper) writeThrough(p []byte) (int, error) {
	i := bytes.IndexByte(p, '\n')
	if i < 0 {
		return e.writeAll(p)
	}

	n, err := e.writeAll(p[:i+1])
	if err != nil {
		return n, err
	}
	e.atLineStart = true

	return n, nil
}

// writeLineStart classifies the start of the line beginning at p, escaping it
// when it would parse as a marker. It returns the number of bytes of p
// consumed; bytes of an undecided line start stay buffered in e.pending.
func (e *Escaper) writeLineStart(p []byte) (int, error) {
	if len(e.pending) == 0 {
		return e.classifyFirstByte(p)
	}

	// pending holds a proper prefix of MarkerPrefix.
	if b := p[0]; b != MarkerPrefix[len(e.pending)] {
		// The line diverged from the marker prefix: it is plain content.
		if _, err := e.writeAll(e.pending); err != nil {
			return 0, err
		}
		e.pending = nil
		e.atLineStart = false

		return 0, nil
	}

	e.pending = append(e.pending, p[0])
	if len(e.pending) < len(MarkerPrefix) {
		return 1, nil
	}

	if _, err := e.writeAll([]byte{escapeByte}); err != nil {
		return 1, err
	}
	if _, err := e.writeAll(e.pending); err != nil {
		return 1, err
	}
	e.pending = nil
	e.atLineStart = false

	return 1, nil
}

// classifyFirstByte decides what the first byte of a line means. No byte is
// consumed unless it is buffered as an undecided line start or the line is
// known to need no escaping.
func (e *Escaper) classifyFirstByte(p []byte) (int, error) {
	switch {
	case p[0] == escapeByte:
		// Escape the escape byte, then pass the original line through.
		if _, err := e.writeAll([]byte{escapeByte}); err != nil {
			return 0, err
		}
		e.atLineStart = false

		return 0, nil
	case p[0] != MarkerPrefix[0]:
		// A plain content line: pass it through untouched.
		e.atLineStart = false

		return 0, nil
	default:
		e.pending = append(e.pending, p[0])

		return 1, nil
	}
}

// SplitToDir reads a logical backup stream and writes the dump of every
// database into its own <database>.sql file inside dir, which must already
// exist. Dump files are opened through an [os.Root] anchored at dir and are
// only created or truncated when the existing target, if any, is a regular
// file, so a symbolic link planted in dir can never redirect the write.
// Dump content is streamed in bounded chunks; only marker lines are buffered.
func SplitToDir(r io.Reader, dir string) error {
	root, openErr := os.OpenRoot(dir)
	if openErr != nil {
		return fmt.Errorf("failed to open target directory: %w", openErr)
	}

	s := &splitter{
		root:      root,
		current:   nil,
		seen:      make(map[string]struct{}),
		sawMarker: false,
		state:     stateLineStart,
		pending:   nil,
	}
	defer func() {
		_ = s.close()
		_ = s.root.Close()
	}()

	reader := bufio.NewReader(r)
	for {
		chunk, readErr := reader.ReadSlice('\n')

		if len(chunk) > 0 {
			if err := s.feed(chunk); err != nil {
				return err
			}
		}

		if readErr == nil || errors.Is(readErr, bufio.ErrBufferFull) {
			// ErrBufferFull means a line longer than the buffer: the chunk
			// holds its first part and the next read continues the line.
			continue
		}
		if errors.Is(readErr, io.EOF) {
			break
		}

		return fmt.Errorf("failed to read stream: %w", readErr)
	}

	if err := s.finish(); err != nil {
		return err
	}

	if err := s.close(); err != nil {
		return err
	}

	if !s.sawMarker {
		return fmt.Errorf("%w", ErrNoMarker)
	}

	return nil
}

// splitState is the position of the reader within the stream.
type splitState int

const (
	// stateLineStart classifies the beginning of the current line.
	stateLineStart splitState = iota
	// stateContent streams a content line into the current dump file.
	stateContent
	// stateMarker buffers a marker line until its terminating newline.
	stateMarker
)

// splitter writes the lines of a logical backup stream to per-database files.
type splitter struct {
	root      *os.Root
	current   *os.File
	seen      map[string]struct{}
	sawMarker bool

	state   splitState
	pending []byte // line-start bytes while undecided; the marker line in stateMarker
}

// feed consumes the next chunk of the stream. A chunk begins at a line start
// or continues the current line; a newline inside the chunk ends the line.
func (s *splitter) feed(chunk []byte) error {
	for len(chunk) > 0 {
		var (
			rest []byte
			err  error
		)
		switch s.state {
		case stateLineStart:
			rest, err = s.classify(chunk)
		case stateContent:
			rest, err = s.writeContentThrough(chunk)
		case stateMarker:
			rest, err = s.bufferMarker(chunk)
		}
		if err != nil {
			return err
		}
		chunk = rest
	}

	return nil
}

// classify consumes the line-start bytes of chunk until the line is known to
// be a marker, escaped content, or plain content, then flushes content bytes
// into the current dump file. It returns the rest of the chunk.
func (s *splitter) classify(chunk []byte) ([]byte, error) {
	strip := false
classifyLoop:
	for len(chunk) > 0 {
		b := chunk[0]
		pos := len(s.pending)
		if pos > 0 && s.pending[0] == escapeByte {
			pos--
		}

		switch {
		case len(s.pending) == 0 && b == escapeByte:
			// Undecided: escaped content or a raw escape-byte line.
			s.pending = append(s.pending, b)
			chunk = chunk[1:]
		case pos < len(MarkerPrefix) && b == MarkerPrefix[pos]:
			s.pending = append(s.pending, b)
			chunk = chunk[1:]
			if pos+1 < len(MarkerPrefix) {
				continue
			}
			if s.pending[0] != escapeByte {
				s.state = stateMarker
				break classifyLoop
			}
			// Content whose producer escaped the marker prefix.
			strip = true
			s.state = stateContent
			break classifyLoop
		case len(s.pending) == 1 && s.pending[0] == escapeByte && b == escapeByte:
			// Content whose producer escaped the escape byte.
			strip = true
			s.state = stateContent
			break classifyLoop
		default:
			// Plain content.
			s.state = stateContent
			break classifyLoop
		}
	}

	if s.state == stateContent {
		return chunk, s.flushLineStart(strip)
	}

	return chunk, nil
}

// flushLineStart writes the held bytes of a classified content line.
func (s *splitter) flushLineStart(strip bool) error {
	p := s.pending
	if strip {
		p = p[1:]
	}
	s.pending = nil

	if !s.sawMarker {
		return fmt.Errorf("%w", ErrUnexpectedContent)
	}
	if len(p) == 0 {
		return nil
	}

	return s.writeContent(p)
}

// writeContentThrough writes the content bytes of chunk up to and including
// the next newline, which ends the line. It returns the rest of the chunk.
func (s *splitter) writeContentThrough(chunk []byte) ([]byte, error) {
	i := bytes.IndexByte(chunk, '\n')
	if i < 0 {
		if err := s.writeContent(chunk); err != nil {
			return nil, err
		}

		return nil, nil
	}

	if err := s.writeContent(chunk[:i+1]); err != nil {
		return nil, err
	}
	s.state = stateLineStart

	return chunk[i+1:], nil
}

// bufferMarker appends the marker-line bytes of chunk, opening the database
// file at the terminating newline. It returns the rest of the chunk.
func (s *splitter) bufferMarker(chunk []byte) ([]byte, error) {
	i := bytes.IndexByte(chunk, '\n')
	n := len(chunk)
	if i >= 0 {
		n = i + 1
	}
	if len(s.pending)+n > maxMarkerLineLen {
		return nil, fmt.Errorf("%w: marker line exceeds %d bytes", ErrInvalidDatabaseName, maxMarkerLineLen)
	}

	if i < 0 {
		s.pending = append(s.pending, chunk...)

		return nil, nil
	}

	s.pending = append(s.pending, chunk[:i+1]...)
	if err := s.openDatabase(string(s.pending)); err != nil {
		return nil, err
	}
	s.pending = nil
	s.state = stateLineStart

	return chunk[i+1:], nil
}

// finish processes a final line that the stream ended without a newline.
func (s *splitter) finish() error {
	switch s.state {
	case stateLineStart:
		if len(s.pending) == 0 {
			return nil
		}
		if !s.sawMarker {
			return fmt.Errorf("%w", ErrUnexpectedContent)
		}
		if err := s.writeContent(s.pending); err != nil {
			return err
		}
		s.pending = nil
	case stateMarker:
		if err := s.openDatabase(string(s.pending)); err != nil {
			return err
		}
		s.pending = nil
	case stateContent:
		// Written as it was read.
	}

	return nil
}

// openDatabase opens the dump file of the database named by a marker line.
func (s *splitter) openDatabase(line string) error {
	db, _ := ParseMarker(line)

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

	name := db + dumpFileSuffix
	if info, statErr := s.root.Lstat(name); statErr == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("%w: %q", ErrNonRegularTarget, name)
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("failed to inspect dump file: %w", statErr)
	}

	file, err := s.root.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, dumpFilePerm)
	if err != nil {
		return fmt.Errorf("failed to create dump file: %w", err)
	}
	s.current = file
	s.sawMarker = true

	return nil
}

// writeContent writes a piece of a content line to the current dump file.
func (s *splitter) writeContent(p []byte) error {
	if _, err := s.current.Write(p); err != nil {
		return fmt.Errorf("failed to write dump file: %w", err)
	}

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
