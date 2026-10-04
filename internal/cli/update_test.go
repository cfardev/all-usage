package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssetName(t *testing.T) {
	cases := []struct{ os, arch, want string }{
		{"linux", "amd64", "all-usage_linux_amd64.tar.gz"},
		{"linux", "arm64", "all-usage_linux_arm64.tar.gz"},
		{"darwin", "amd64", "all-usage_darwin_amd64.tar.gz"},
		{"darwin", "arm64", "all-usage_darwin_arm64.tar.gz"},
		{"windows", "amd64", "all-usage_windows_amd64.zip"},
		{"windows", "arm64", "all-usage_windows_arm64.zip"},
	}
	for _, tc := range cases {
		got, err := assetName(tc.os, tc.arch)
		if err != nil || got != tc.want {
			t.Errorf("assetName(%s, %s) = %q, %v", tc.os, tc.arch, got, err)
		}
	}
	if _, err := assetName("freebsd", "amd64"); err == nil {
		t.Fatal("expected unsupported OS")
	}
	if _, err := assetName("linux", "386"); err == nil {
		t.Fatal("expected unsupported architecture")
	}
}

func TestSameRelease(t *testing.T) {
	if !sameRelease("v0.1.0", "v0.1.0") || !sameRelease("0.1.0", "v0.1.0") {
		t.Fatal("matching versions should be current")
	}
	for _, local := range []string{"dev", "(devel)", "", "v0.1.0", "v0.0.0-20261003120000-abcdef"} {
		if sameRelease(local, "v0.1.1") {
			t.Errorf("sameRelease(%q, v0.1.1) = true", local)
		}
	}
}

func TestUpdateRelease(t *testing.T) {
	archive := tarGz(t, "all-usage", "new-binary")
	const tag = "v9.9.9"
	asset := "all-usage_linux_amd64.tar.gz"
	sum := sha256.Sum256(archive)
	checksums := hex.EncodeToString(sum[:]) + "  " + asset + "\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/latest":
			_, _ = w.Write([]byte(`{"tag_name":"` + tag + `"}`))
		case "/dl/" + tag + "/" + asset:
			_, _ = w.Write(archive)
		case "/dl/" + tag + "/checksums.txt":
			_, _ = w.Write([]byte(checksums))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	exe := filepath.Join(t.TempDir(), "all-usage")
	if err := os.WriteFile(exe, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := updateRelease(context.Background(), srv.Client(), updateOpts{
		API:          srv.URL + "/api/latest",
		DownloadBase: srv.URL + "/dl",
		Version:      "dev",
		GOOS:         "linux",
		GOARCH:       "amd64",
		Exe:          exe,
		Out:          &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new-binary" {
		t.Fatalf("binary = %q", got)
	}
	if !strings.Contains(out.String(), "Updated all-usage dev -> v9.9.9") {
		t.Fatalf("output = %q", out.String())
	}
	info, err := os.Stat(exe)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestUpdateAlreadyCurrent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/latest" {
			t.Errorf("unexpected download %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.3"}`))
	}))
	defer srv.Close()

	exe := filepath.Join(t.TempDir(), "all-usage")
	if err := os.WriteFile(exe, []byte("keep"), 0o755); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := updateRelease(context.Background(), srv.Client(), updateOpts{
		API:          srv.URL + "/api/latest",
		DownloadBase: srv.URL + "/dl",
		Version:      "v1.2.3",
		GOOS:         "linux",
		GOARCH:       "amd64",
		Exe:          exe,
		Out:          &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "keep" || !strings.Contains(out.String(), "up to date") {
		t.Fatalf("body %q output %q", got, out.String())
	}
}

func TestUpdateNoRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	err := updateRelease(context.Background(), srv.Client(), updateOpts{
		API:          srv.URL + "/api/latest",
		DownloadBase: srv.URL + "/dl",
		Version:      "dev",
		GOOS:         "linux",
		GOARCH:       "amd64",
		Exe:          filepath.Join(t.TempDir(), "all-usage"),
	})
	if err == nil || !strings.Contains(err.Error(), "no GitHub release found") {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdateChecksumMismatch(t *testing.T) {
	archive := tarGz(t, "all-usage", "new-binary")
	const tag = "v9.9.9"
	asset := "all-usage_linux_amd64.tar.gz"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/latest":
			_, _ = w.Write([]byte(`{"tag_name":"` + tag + `"}`))
		case "/dl/" + tag + "/" + asset:
			_, _ = w.Write(archive)
		case "/dl/" + tag + "/checksums.txt":
			_, _ = w.Write([]byte("deadbeef  " + asset + "\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	exe := filepath.Join(t.TempDir(), "all-usage")
	if err := os.WriteFile(exe, []byte("old-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	err := updateRelease(context.Background(), srv.Client(), updateOpts{
		API:          srv.URL + "/api/latest",
		DownloadBase: srv.URL + "/dl",
		Version:      "dev",
		GOOS:         "linux",
		GOARCH:       "amd64",
		Exe:          exe,
	})
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("err = %v", err)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "old-binary" {
		t.Fatalf("binary was replaced: %q", got)
	}
}

func TestReplaceBinaryWindows(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "all-usage.exe")
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := replaceBinary("windows", exe, []byte("new")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(exe)
	old, _ := os.ReadFile(exe + ".old")
	if string(got) != "new" || string(old) != "old" {
		t.Fatalf("exe %q old %q", got, old)
	}
}

func TestExtractRejectsOtherNames(t *testing.T) {
	archive := tarGz(t, "../all-usage", "nope")
	if _, err := extractBinary(archive, "all-usage_linux_amd64.tar.gz", "all-usage"); err == nil {
		t.Fatal("expected archive without all-usage to fail")
	}
}

func TestExtractZip(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("all-usage.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("from-zip")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := extractBinary(buf.Bytes(), "all-usage_windows_amd64.zip", "all-usage.exe")
	if err != nil || string(got) != "from-zip" {
		t.Fatalf("extract = %q, %v", got, err)
	}
}

func tarGz(t *testing.T, name, body string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	b := []byte(body)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(b))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(b); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
