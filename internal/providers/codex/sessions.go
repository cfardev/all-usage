package codex

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cfardev/all-usage/internal/jsonx"
)

// sessionLine is a rollout-*.jsonl event carrying rate limits:
// {"timestamp":..., "type":"event_msg", "payload":{"type":"token_count", "rate_limits":{...}}}
type sessionLine struct {
	Timestamp string `json:"timestamp"`
	Payload   struct {
		RateLimits *struct {
			LimitID   jsonx.String    `json:"limit_id"`
			Primary   *sessionWindow  `json:"primary"`
			Secondary *sessionWindow  `json:"secondary"`
			Credits   *whamCredits    `json:"credits"`
			PlanType  jsonx.String    `json:"plan_type"`
			Reached   json.RawMessage `json:"rate_limit_reached_type"`
		} `json:"rate_limits"`
	} `json:"payload"`
}

type sessionWindow struct {
	UsedPercent     float64      `json:"used_percent"`
	WindowMinutes   jsonx.Number `json:"window_minutes"`
	ResetsAt        jsonx.Number `json:"resets_at"`
	ResetsInSeconds jsonx.Number `json:"resets_in_seconds"` // older Codex versions
}

func (w *sessionWindow) toWindow(at time.Time) *window {
	if w == nil {
		return nil
	}
	out := &window{UsedPercent: w.UsedPercent, Length: time.Duration(w.WindowMinutes.Or(0)) * time.Minute}
	switch {
	case w.ResetsAt.Or(0) > 0:
		out.ResetsAt = jsonx.UnixTime(w.ResetsAt.V)
	case w.ResetsInSeconds.Valid && !at.IsZero():
		out.ResetsAt = at.Add(time.Duration(w.ResetsInSeconds.V) * time.Second)
	}
	return out
}

type sessionFile struct {
	Path    string
	ModTime time.Time
}

// sessionFiles returns the most recently modified rollout logs (newest first)
// across all Codex homes, skipping day directories older than maxAge.
func sessionFiles(homes []home, now time.Time, maxAge time.Duration, limit int) []sessionFile {
	cutoff := now.Add(-maxAge)
	var files []sessionFile
	for _, h := range homes {
		root := filepath.Join(h.Dir, "sessions")
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(root, path)
			parts := strings.Split(filepath.ToSlash(rel), "/")
			if d.IsDir() {
				if path != root && dirTooOld(parts, cutoff) {
					return filepath.SkipDir
				}
				return nil
			}
			name := d.Name()
			if !strings.HasPrefix(name, "rollout-") || !strings.HasSuffix(name, ".jsonl") {
				return nil
			}
			if info, err := d.Info(); err == nil && info.ModTime().After(cutoff) {
				files = append(files, sessionFile{Path: path, ModTime: info.ModTime()})
			}
			return nil
		})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].ModTime.After(files[j].ModTime) })
	if len(files) > limit {
		files = files[:limit]
	}
	return files
}

// dirTooOld reports whether a sessions/YYYY[/MM[/DD]] directory only holds
// sessions that started before cutoff. Sessions can stay active for days, so a
// week of slack is allowed.
func dirTooOld(parts []string, cutoff time.Time) bool {
	nums := make([]int, 0, 3)
	for _, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return false
		}
		nums = append(nums, n)
	}
	var end time.Time
	switch len(nums) {
	case 1:
		end = time.Date(nums[0]+1, 1, 1, 0, 0, 0, 0, time.Local)
	case 2:
		end = time.Date(nums[0], time.Month(nums[1])+1, 1, 0, 0, 0, 0, time.Local)
	case 3:
		end = time.Date(nums[0], time.Month(nums[1]), nums[2]+1, 0, 0, 0, 0, time.Local)
	default:
		return false
	}
	return end.Add(7 * 24 * time.Hour).Before(cutoff)
}

var rateLimitsKey = []byte(`"rate_limits"`)

// lastRateLimits returns the most recent rate-limit snapshot in a rollout log,
// reading the file backwards in growing chunks (logs can be very large).
func lastRateLimits(path string) (*snapshot, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := st.Size()
	var other *snapshot // snapshot of a secondary (per-model) limit, used if no main one exists
	for chunk := int64(1 << 20); ; chunk *= 4 {
		n := min(chunk, size)
		buf := make([]byte, n)
		if _, err := f.ReadAt(buf, size-n); err != nil {
			return nil, err
		}
		lines := bytes.Split(buf, []byte{'\n'})
		first := 0
		if n < size {
			first = 1 // the first line is probably cut
		}
		for i := len(lines) - 1; i >= first; i-- {
			if !bytes.Contains(lines[i], rateLimitsKey) {
				continue
			}
			s := parseSessionLine(lines[i], st.ModTime())
			if s == nil {
				continue
			}
			if id := s.Groups[0].ID; id == "codex" {
				return s, nil
			}
			if other == nil {
				other = s
			}
		}
		if n == size || n >= 64<<20 {
			break
		}
	}
	if other != nil {
		return other, nil
	}
	return nil, errors.New("no rate limits recorded")
}

func parseSessionLine(line []byte, fallback time.Time) *snapshot {
	var l sessionLine
	if json.Unmarshal(line, &l) != nil || l.Payload.RateLimits == nil {
		return nil
	}
	rl := l.Payload.RateLimits
	if rl.Primary == nil && rl.Secondary == nil {
		return nil
	}
	at, ok := jsonx.ParseTime(l.Timestamp)
	if !ok {
		at = fallback
	}
	s := &snapshot{Plan: rl.PlanType.V, Reached: parseReached(rl.Reached), AsOf: at, Credits: rl.Credits.toCredits()}
	id := rl.LimitID.V
	if id == "" {
		id = "codex"
	}
	s.Groups = []limitGroup{{ID: id, Primary: rl.Primary.toWindow(at), Secondary: rl.Secondary.toWindow(at)}}
	return s
}
