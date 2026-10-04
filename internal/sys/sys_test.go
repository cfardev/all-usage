package sys_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/cfardev/all-usage/internal/sys"
	"github.com/cfardev/all-usage/internal/testutil"
)

func TestReadKV(t *testing.T) {
	// Path characters that need escaping in SQLite URIs.
	db := filepath.Join(t.TempDir(), "dir with spaces #1 ?x", "state.vscdb")
	testutil.KVDB(t, db, map[string]map[string]string{"ItemTable": {"a": "1", "b": `{"x":2}`}})
	before, _ := os.Stat(db)

	kv, err := sys.ReadKV(context.Background(), db, "ItemTable", "a", "b", "missing")
	if err != nil {
		t.Fatal(err)
	}
	if kv["a"] != "1" || kv["b"] != `{"x":2}` || len(kv) != 2 {
		t.Fatalf("kv = %v", kv)
	}
	after, _ := os.Stat(db)
	if !after.ModTime().Equal(before.ModTime()) || after.Size() != before.Size() {
		t.Error("ReadKV must not modify the database")
	}
	if _, err := sys.ReadKV(context.Background(), db, "ItemTable; DROP TABLE x", "a"); err == nil {
		t.Error("invalid table names must be rejected")
	}
	if _, err := sys.ReadKV(context.Background(), db, "nope", "a"); err == nil {
		t.Error("missing table must fail")
	}
	if _, err := sys.ReadKV(context.Background(), filepath.Join(t.TempDir(), "missing.db"), "ItemTable", "a"); !os.IsNotExist(err) {
		t.Errorf("missing file: %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(filepath.Dir(db), "*"))
	if len(matches) != 1 {
		t.Errorf("reading must not create journal files: %v", matches)
	}
}

func TestJWT(t *testing.T) {
	exp := time.Now().Add(time.Hour).Truncate(time.Second)
	tok := testutil.JWT(map[string]any{"exp": exp.Unix(), "ns": map[string]any{"plan": "pro"}})
	got, ok := sys.JWTExpiry(tok)
	if !ok || !got.Equal(exp) {
		t.Errorf("expiry = %v %v", got, ok)
	}
	claims, err := sys.JWTClaims(tok)
	if err != nil || sys.ClaimString(claims, "ns", "plan") != "pro" || sys.ClaimString(claims, "ns", "nope") != "" {
		t.Errorf("claims = %v %v", claims, err)
	}
	if _, err := sys.JWTClaims("not.a.jwt!"); err == nil {
		t.Error("expected error")
	}
	if _, ok := sys.JWTExpiry("abc"); ok {
		t.Error("expected no expiry")
	}
}

func TestWindowsHomesOverride(t *testing.T) {
	if got := sys.WindowsHomes("/custom/home", true); len(got) != 1 || got[0] != "/custom/home" {
		t.Errorf("got %v", got)
	}
	if got := sys.WindowsHomes("", false); got != nil {
		t.Errorf("scan disabled: %v", got)
	}
}

func TestExpandPath(t *testing.T) {
	home := sys.Home()
	t.Setenv("ALL_USAGE_TEST_DIR", "/x/y")
	for in, want := range map[string]string{"~": home, "~/a/b": filepath.Join(home, "a", "b"), "$ALL_USAGE_TEST_DIR/z": "/x/y/z", "": "", " /abs ": "/abs"} {
		if got := sys.ExpandPath(in); got != want {
			t.Errorf("ExpandPath(%q) = %q, want %q", in, got, want)
		}
	}
	if got := sys.ShortPath(filepath.Join(home, "f")); got != "~"+string(os.PathSeparator)+"f" {
		t.Errorf("ShortPath = %q", got)
	}
}

func TestFindBinary(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}
	dir := t.TempDir()
	bin := testutil.Script(t, dir, "tool", "exit 0\n")
	if got, ok := sys.FindBinary(bin); !ok || got != bin {
		t.Errorf("absolute path: %q %v", got, ok)
	}
	t.Setenv("PATH", dir)
	if got, ok := sys.FindBinary("tool"); !ok || got != bin {
		t.Errorf("PATH lookup: %q %v", got, ok)
	}
	if _, ok := sys.FindBinary("definitely-not-installed-xyz"); ok {
		t.Error("expected not found")
	}
	plain := filepath.Join(dir, "plain")
	testutil.WriteFile(t, plain, "data")
	if _, ok := sys.FindBinary(plain); ok {
		t.Error("non-executable file must not be found")
	}
}
