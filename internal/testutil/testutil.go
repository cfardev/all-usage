// Package testutil holds helpers shared by tests: fake JWTs, SQLite
// key/value fixtures and fake executables. It is only imported by tests.
package testutil

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	_ "modernc.org/sqlite"
)

// JWT returns an unsigned token whose payload is claims.
func JWT(claims map[string]any) string {
	b, err := json.Marshal(claims)
	if err != nil {
		panic(err)
	}
	enc := base64.RawURLEncoding.EncodeToString
	return enc([]byte(`{"alg":"none","typ":"JWT"}`)) + "." + enc(b) + ".c2ln"
}

// KVDB creates a SQLite database at path with one key/value table per entry
// of tables (table -> key -> value). The database is built at a plain
// temporary path and then copied, so path may contain URI-special characters.
func KVDB(t testing.TB, path string, tables map[string]map[string]string) {
	t.Helper()
	tmp := filepath.Join(t.TempDir(), "kv.db")
	db, err := sql.Open("sqlite", tmp)
	if err != nil {
		t.Fatal(err)
	}
	for table, kv := range tables {
		if _, err := db.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %q (key TEXT PRIMARY KEY, value BLOB)`, table)); err != nil {
			t.Fatal(err)
		}
		for k, v := range kv {
			if _, err := db.Exec(fmt.Sprintf(`INSERT OR REPLACE INTO %q (key, value) VALUES (?, ?)`, table), k, v); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

// Script writes an executable POSIX shell script and returns its path. Tests
// using it are skipped on Windows.
func Script(t testing.TB, dir, name, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts are not supported on Windows")
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// WriteFile writes content to path, creating parent directories.
func WriteFile(t testing.TB, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
