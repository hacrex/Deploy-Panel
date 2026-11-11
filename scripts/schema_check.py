#!/usr/bin/env python3
"""Apply every migration in order to an in-memory SQLite database and lint them.

Checks:
  - all migrations apply cleanly, in order, in one database
  - no foreign key violations
  - migrations avoid constructs that would break PostgreSQL portability

The Go tests in internal/store cover the real runner; this script needs no CGO or Go
and is handy in CI and on machines without a C compiler.
"""
import pathlib
import re
import sqlite3
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent
FILES = sorted((ROOT / "migrations").glob("[0-9][0-9][0-9][0-9]_*.sql"))

# Constructs that work in SQLite but not in PostgreSQL (or behave differently).
FORBIDDEN = [
    (r"\bPRAGMA\b", "PRAGMA is per connection; set it in internal/store/driver.go"),
    (r"\bAUTOINCREMENT\b", "use ULID text ids instead"),
    (r"\bINSERT\s+OR\b", "use INSERT ... ON CONFLICT instead"),
    (r"\bWITHOUT\s+ROWID\b", "SQLite only"),
    (r"\bGLOB\b", "SQLite only"),
]


def strip_comments(sql: str) -> str:
    return re.sub(r"--[^\n]*", "", sql)


def main() -> int:
    if not FILES:
        print("no migrations found", file=sys.stderr)
        return 1

    problems = []
    for f in FILES:
        body = strip_comments(f.read_text())
        for pattern, why in FORBIDDEN:
            if re.search(pattern, body, re.IGNORECASE):
                problems.append(f"{f.name}: {pattern} found ({why})")

    conn = sqlite3.connect(":memory:")
    conn.execute("PRAGMA foreign_keys = ON")
    for f in FILES:
        try:
            conn.executescript(f.read_text())
        except sqlite3.Error as exc:
            print(f"FAIL {f.name}: {exc}", file=sys.stderr)
            return 1
        print("ok  ", f.name)

    violations = conn.execute("PRAGMA foreign_key_check").fetchall()
    if violations:
        problems.append(f"foreign key violations: {violations}")

    tables = [r[0] for r in conn.execute(
        "SELECT name FROM sqlite_master WHERE type = 'table' ORDER BY name")]
    print(f"{len(tables)} tables")

    for p in problems:
        print("FAIL", p, file=sys.stderr)
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main())
