package sys

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	_ "modernc.org/sqlite" // pure-Go SQLite driver (no cgo)
)

var identRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// ReadKV returns the `value` column for each of keys from a key/value table
// (columns `key`, `value`) of the SQLite database at path. The database is
// always opened read-only. Missing keys are omitted from the result.
//
// It first opens the file with normal locking; if that fails (for example on
// Windows drives mounted in WSL, where POSIX locks are unreliable) it retries
// in immutable mode, which never takes locks nor writes.
func ReadKV(ctx context.Context, path, table string, keys ...string) (map[string]string, error) {
	if !identRe.MatchString(table) {
		return nil, fmt.Errorf("invalid table name %q", table)
	}
	if !FileExists(path) {
		return nil, &fs.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
	}
	out, err := readKV(ctx, sqliteURI(path, "mode=ro", "_pragma=busy_timeout(2000)"), table, keys)
	if err == nil {
		return out, nil
	}
	if out2, err2 := readKV(ctx, sqliteURI(path, "mode=ro", "immutable=1"), table, keys); err2 == nil {
		return out2, nil
	}
	return nil, fmt.Errorf("read %s: %w", path, err)
}

func readKV(ctx context.Context, dsn, table string, keys []string) (map[string]string, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	out := make(map[string]string, len(keys))
	if len(keys) == 0 {
		return out, db.PingContext(ctx)
	}
	args := make([]any, len(keys))
	for i, k := range keys {
		args[i] = k
	}
	// The table name is validated above; keys are bound parameters.
	q := fmt.Sprintf(`SELECT key, value FROM %q WHERE key IN (?%s)`, table, strings.Repeat(",?", len(keys)-1))
	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var v any
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		switch t := v.(type) {
		case string:
			out[k] = t
		case []byte:
			out[k] = string(t)
		case nil:
		default:
			out[k] = fmt.Sprint(t)
		}
	}
	return out, rows.Err()
}

// sqliteURI builds a SQLite URI filename (https://sqlite.org/uri.html).
func sqliteURI(path string, params ...string) string {
	p := filepath.ToSlash(path)
	if runtime.GOOS == "windows" && len(p) > 1 && p[1] == ':' {
		p = "/" + p
	}
	var b strings.Builder
	b.WriteString("file:")
	for _, r := range p {
		switch r {
		case '?':
			b.WriteString("%3f")
		case '#':
			b.WriteString("%23")
		case '%':
			b.WriteString("%25")
		default:
			b.WriteRune(r)
		}
	}
	if len(params) > 0 {
		b.WriteByte('?')
		b.WriteString(strings.Join(params, "&"))
	}
	return b.String()
}
