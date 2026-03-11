package registry

import (
	"strings"
	"time"
)

func backupTimeFromFilename(filename string) (time.Time, bool) {
	name := strings.TrimSuffix(filename, ".enc")
	name = strings.TrimSuffix(name, ".tar.gz")
	tm, err := time.ParseInLocation("2006-01-02-15-04-05", name, time.UTC)
	if err != nil {
		return time.Time{}, false
	}

	return tm.UTC(), true
}
