package store

// This is the only file that knows which SQLite driver is in use.
//
// Current choice: github.com/mattn/go-sqlite3. It needs CGO, so dpd is not a
// fully static binary yet (see docs/adr/0001-single-binary-go.md).
// To move to a pure-Go driver such as modernc.org/sqlite, change the import,
// driverName and dsn below; nothing else in the repo depends on the driver.

import (
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

const driverName = "sqlite3"

var pathEscaper = strings.NewReplacer("?", "%3f", "#", "%23")

// dsn enables foreign keys, WAL, a busy timeout and immediate write locks on every connection.
func dsn(path string) string {
	return "file:" + pathEscaper.Replace(path) +
		"?_foreign_keys=1&_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_txlock=immediate"
}
