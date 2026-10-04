package cli

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/cfardev/all-usage/internal/buildinfo"
)

const (
	releaseRepo    = "cfardev/all-usage"
	releaseAPI     = "https://api.github.com/repos/" + releaseRepo + "/releases/latest"
	releaseBase    = "https://github.com/" + releaseRepo + "/releases/download"
	maxReleaseSize = 64 << 20
)

var (
	releaseHTTP = &http.Client{Timeout: 5 * time.Minute}
	tagRE       = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)
)

// assetName is the release archive for an OS and architecture.
// Names match scripts/dist.sh and install.sh.
func assetName(goos, goarch string) (string, error) {
	switch goarch {
	case "amd64", "arm64":
	default:
		return "", fmt.Errorf("unsupported architecture %s", goarch)
	}
	switch goos {
	case "linux", "darwin":
		return fmt.Sprintf("all-usage_%s_%s.tar.gz", goos, goarch), nil
	case "windows":
		return fmt.Sprintf("all-usage_%s_%s.zip", goos, goarch), nil
	default:
		return "", fmt.Errorf("unsupported OS %s", goos)
	}
}

func binName(goos string) string {
	if goos == "windows" {
		return "all-usage.exe"
	}
	return "all-usage"
}

// sameRelease reports whether the local version is already the published tag.
// dev builds and empty versions are never current.
func sameRelease(local, tag string) bool {
	local = strings.TrimPrefix(strings.TrimSpace(local), "v")
	tag = strings.TrimPrefix(strings.TrimSpace(tag), "v")
	switch local {
	case "", "dev", "(devel)":
		return false
	}
	return local == tag
}

type updateOpts struct {
	API          string
	DownloadBase string
	Version      string
	GOOS         string
	GOARCH       string
	Exe          string
	Out          io.Writer
}

func newUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "Replace this binary with the latest GitHub release",
		Long: `Download the latest all-usage release for this OS and architecture,
check its sha256 against the release checksums, and replace the current
executable. A binary that is already that version is left as-is.

If the install directory is not writable, re-run the install script.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUpdate(cmd.Context(), cmd.OutOrStdout())
		},
	}
}

func runUpdate(ctx context.Context, out io.Writer) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return updateRelease(ctx, releaseHTTP, updateOpts{
		API:          releaseAPI,
		DownloadBase: releaseBase,
		Version:      buildinfo.String(),
		GOOS:         runtime.GOOS,
		GOARCH:       runtime.GOARCH,
		Exe:          exe,
		Out:          out,
	})
}

func updateRelease(ctx context.Context, client *http.Client, o updateOpts) error {
	if client == nil {
		client = releaseHTTP
	}
	if o.Out == nil {
		o.Out = io.Discard
	}
	asset, err := assetName(o.GOOS, o.GOARCH)
	if err != nil {
		return err
	}
	tag, err := latestTag(ctx, client, o.API)
	if err != nil {
		return err
	}
	if sameRelease(o.Version, tag) {
		fmt.Fprintf(o.Out, "all-usage %s is up to date\n", tag)
		return nil
	}
	if err := canWrite(o.Exe); err != nil {
		return replaceErr(o.Exe, err)
	}

	base := strings.TrimRight(o.DownloadBase, "/")
	archive, err := fetch(ctx, client, base+"/"+tag+"/"+asset, maxReleaseSize, false)
	if err != nil {
		return err
	}
	sums, err := fetch(ctx, client, base+"/"+tag+"/checksums.txt", 1<<20, false)
	if err != nil {
		return err
	}
	if err := verifyChecksum(sums, asset, archive); err != nil {
		return err
	}
	bin, err := extractBinary(archive, asset, binName(o.GOOS))
	if err != nil {
		return err
	}
	if err := replaceBinary(o.GOOS, o.Exe, bin); err != nil {
		return err
	}
	fmt.Fprintf(o.Out, "Updated all-usage %s -> %s\n", o.Version, tag)
	return nil
}

func latestTag(ctx context.Context, client *http.Client, api string) (string, error) {
	body, err := fetch(ctx, client, api, 1<<20, true)
	if err != nil {
		return "", err
	}
	var rel struct {
		TagName string `json:"tag_name"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &rel); err != nil {
		return "", fmt.Errorf("latest release: %w", err)
	}
	if rel.TagName == "" {
		if rel.Message != "" {
			return "", fmt.Errorf("github: %s", rel.Message)
		}
		return "", fmt.Errorf("no GitHub release found")
	}
	if !tagRE.MatchString(rel.TagName) {
		return "", fmt.Errorf("unexpected release tag %q", rel.TagName)
	}
	return rel.TagName, nil
}

func fetch(ctx context.Context, client *http.Client, url string, limit int64, api bool) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", buildinfo.UserAgent())
	if api {
		req.Header.Set("Accept", "application/vnd.github+json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("GET %s: larger than %d bytes", url, limit)
	}
	if resp.StatusCode == http.StatusNotFound && api {
		return nil, fmt.Errorf("no GitHub release found")
	}
	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(data))
		if msg == "" {
			msg = resp.Status
		}
		return nil, fmt.Errorf("GET %s: %s", url, msg)
	}
	return data, nil
}

func verifyChecksum(checksums []byte, name string, data []byte) error {
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	for _, line := range strings.Split(string(checksums), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == name {
			if !strings.EqualFold(fields[0], got) {
				return fmt.Errorf("checksum mismatch for %s", name)
			}
			return nil
		}
	}
	return fmt.Errorf("no checksum for %s", name)
}

func extractBinary(archive []byte, name, want string) ([]byte, error) {
	switch {
	case strings.HasSuffix(name, ".tar.gz"):
		return extractTar(archive, want)
	case strings.HasSuffix(name, ".zip"):
		return extractZip(archive, want)
	default:
		return nil, fmt.Errorf("unknown archive %s", name)
	}
}

func wantedName(name, want string) bool {
	return name == want || name == "./"+want
}

func extractTar(data []byte, want string) ([]byte, error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if !wantedName(h.Name, want) {
			continue
		}
		return readLimited(tr, maxReleaseSize, want)
	}
	return nil, fmt.Errorf("archive does not contain %s", want)
}

func extractZip(data []byte, want string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	for _, f := range zr.File {
		if !wantedName(f.Name, want) {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		data, err := readLimited(rc, maxReleaseSize, want)
		rc.Close()
		return data, err
	}
	return nil, fmt.Errorf("archive does not contain %s", want)
}

func readLimited(r io.Reader, limit int64, name string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s is too large", name)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("%s is empty", name)
	}
	return data, nil
}

func canWrite(exe string) error {
	tmp := exe + ".new"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Remove(tmp)
}

func replaceBinary(goos, exe string, data []byte) error {
	tmp := exe + ".new"
	if err := os.WriteFile(tmp, data, 0o755); err != nil {
		return replaceErr(exe, err)
	}
	if err := os.Chmod(tmp, 0o755); err != nil {
		os.Remove(tmp)
		return replaceErr(exe, err)
	}
	if goos == "windows" {
		old := exe + ".old"
		_ = os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			os.Remove(tmp)
			return replaceErr(exe, err)
		}
		if err := os.Rename(tmp, exe); err != nil {
			if rb := os.Rename(old, exe); rb != nil {
				return fmt.Errorf("cannot replace %s: %w (restore failed: %v)", exe, err, rb)
			}
			os.Remove(tmp)
			return replaceErr(exe, err)
		}
		return nil
	}
	if err := os.Rename(tmp, exe); err != nil {
		os.Remove(tmp)
		return replaceErr(exe, err)
	}
	return nil
}

func replaceErr(exe string, err error) error {
	return fmt.Errorf("cannot replace %s: %w\nre-run the install script, or update with write permission to that directory", exe, err)
}
