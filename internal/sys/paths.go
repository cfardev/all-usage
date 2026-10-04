// Package sys contains small OS helpers: well-known directories, WSL
// detection, read-only SQLite access, JWT inspection and binary lookup.
package sys

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

// Home returns the current user's home directory.
func Home() string {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return h
	}
	return os.Getenv("HOME")
}

// ExpandPath expands environment variables and a leading "~" in p.
func ExpandPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	p = os.ExpandEnv(p)
	if p == "~" {
		return Home()
	}
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		return filepath.Join(Home(), p[2:])
	}
	return p
}

// ShortPath replaces the home directory prefix of p with "~" for display.
func ShortPath(p string) string {
	h := Home()
	if h != "" && (p == h || strings.HasPrefix(p, h+string(os.PathSeparator))) {
		return "~" + p[len(h):]
	}
	return p
}

// XDGConfigHome returns $XDG_CONFIG_HOME or ~/.config.
func XDGConfigHome() string {
	if v := os.Getenv("XDG_CONFIG_HOME"); v != "" && filepath.IsAbs(v) {
		return v
	}
	return filepath.Join(Home(), ".config")
}

// XDGDataHome returns $XDG_DATA_HOME or ~/.local/share.
func XDGDataHome() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" && filepath.IsAbs(v) {
		return v
	}
	return filepath.Join(Home(), ".local", "share")
}

// MacAppSupport returns ~/Library/Application Support.
func MacAppSupport() string {
	return filepath.Join(Home(), "Library", "Application Support")
}

// WindowsAppData returns %APPDATA% (roaming), falling back to ~/AppData/Roaming.
func WindowsAppData() string {
	if v := os.Getenv("APPDATA"); v != "" {
		return v
	}
	return filepath.Join(Home(), "AppData", "Roaming")
}

// WindowsLocalAppData returns %LOCALAPPDATA%, falling back to ~/AppData/Local.
func WindowsLocalAppData() string {
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		return v
	}
	return filepath.Join(Home(), "AppData", "Local")
}

// AppDataDirs returns the per-platform directories where desktop apps (VS
// Code forks such as Cursor) keep their user data: ~/.config on Linux,
// ~/Library/Application Support on macOS and %APPDATA% on Windows.
func AppDataDirs() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{MacAppSupport()}
	case "windows":
		return []string{WindowsAppData()}
	default:
		return []string{XDGConfigHome()}
	}
}

// LocalDataDir returns where CLI tools written with Rust's `dirs::data_local_dir`
// keep their data: ~/.local/share, ~/Library/Application Support or %LOCALAPPDATA%.
func LocalDataDir() string {
	switch runtime.GOOS {
	case "darwin":
		return MacAppSupport()
	case "windows":
		return WindowsLocalAppData()
	default:
		return XDGDataHome()
	}
}

// FileExists reports whether p exists and is not a directory.
func FileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// DirExists reports whether p exists and is a directory.
func DirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

var (
	wslOnce sync.Once
	wslVal  bool
)

// IsWSL reports whether the process runs inside Windows Subsystem for Linux.
func IsWSL() bool {
	wslOnce.Do(func() {
		if runtime.GOOS != "linux" {
			return
		}
		if os.Getenv("WSL_DISTRO_NAME") != "" || os.Getenv("WSL_INTEROP") != "" {
			wslVal = true
			return
		}
		if b, err := os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
			wslVal = strings.Contains(strings.ToLower(string(b)), "microsoft")
		}
	})
	return wslVal
}

// WindowsHomes returns Windows user profile directories reachable from WSL
// (for example /mnt/c/Users/alice), so credentials of Windows-side apps can be
// discovered. A non-empty override is returned as the only candidate. Outside
// WSL, or when scan is false, it returns nil.
func WindowsHomes(override string, scan bool) []string {
	if override = ExpandPath(override); override != "" {
		return []string{override}
	}
	if !scan || !IsWSL() {
		return nil
	}
	return scanWindowsHomes("/mnt/c/Users", os.Getenv("USER"))
}

var systemProfiles = map[string]bool{
	"all users": true, "default": true, "default user": true, "public": true,
	"defaultuser0": true, "wsiaccount": true, "wdagutilityaccount": true,
}

func scanWindowsHomes(root, user string) []string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var homes []string
	for _, e := range entries {
		if !e.IsDir() || systemProfiles[strings.ToLower(e.Name())] {
			continue
		}
		p := filepath.Join(root, e.Name())
		if DirExists(filepath.Join(p, "AppData")) {
			homes = append(homes, p)
		}
	}
	// Prefer the profile whose name matches the Linux user.
	sort.SliceStable(homes, func(i, j int) bool {
		return strings.EqualFold(filepath.Base(homes[i]), user) && !strings.EqualFold(filepath.Base(homes[j]), user)
	})
	return homes
}
