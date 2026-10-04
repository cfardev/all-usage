package sys

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanWindowsHomes(t *testing.T) {
	root := t.TempDir()
	for _, d := range []string{"Public/AppData", "Default/AppData", "alice/AppData", "Bob/AppData", "nodata"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	got := scanWindowsHomes(root, "bob")
	if len(got) != 2 || filepath.Base(got[0]) != "Bob" || filepath.Base(got[1]) != "alice" {
		t.Fatalf("got %v (want Bob first, system profiles skipped)", got)
	}
	if scanWindowsHomes(filepath.Join(root, "missing"), "x") != nil {
		t.Error("missing root must return nil")
	}
}

func TestSQLiteURI(t *testing.T) {
	got := sqliteURI("/a b/c?d#e%f", "mode=ro", "immutable=1")
	want := "file:/a b/c%3fd%23e%25f?mode=ro&immutable=1"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
