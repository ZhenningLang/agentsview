package main

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/mattn/go-runewidth"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/service"
)

const resumeActiveWindow = 15 * time.Minute

func printSessionListHumanAt(w io.Writer, list *service.SessionList, now time.Time, home string) error {
	if len(list.Sessions) == 0 {
		fmt.Fprintln(w, "(no sessions)")
		return nil
	}
	cols := []string{"", "ID", "AGE", "AGENT", "PROJECT", "BRANCH", "MSGS", "NAME", "CWD"}
	rows := make([][]string, 0, len(list.Sessions)+1)
	rows = append(rows, cols)
	for _, s := range list.Sessions {
		marker := ""
		if sessionRecentlyActive(s, now) {
			marker = "*"
		}
		activity := sessionActivityTime(s)
		rows = append(rows, []string{
			marker,
			sanitizeTerminal(s.ID),
			humanizeSessionAge(activity, now),
			sanitizeTerminal(emptyDash(s.Agent)),
			sanitizeTerminal(emptyDash(s.Project)),
			sanitizeTerminal(emptyDash(s.GitBranch)),
			fmt.Sprintf("%d/%d", s.UserMessageCount, s.MessageCount),
			sanitizeTerminal(truncName(sessionDisplayName(s), 32)),
			sanitizeTerminal(emptyDash(collapseHome(s.Cwd, home))),
		})
	}
	writeAlignedRows(w, rows)
	if list.NextCursor != "" {
		fmt.Fprintf(w, "\nMore sessions: --cursor %s\n", sanitizeTerminal(list.NextCursor))
	}
	return nil
}

func writeAlignedRows(w io.Writer, rows [][]string) {
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			widths[i] = max(widths[i], runewidth.StringWidth(cell))
		}
	}
	for _, row := range rows {
		for i, cell := range row {
			if i > 0 {
				fmt.Fprint(w, "  ")
			}
			fmt.Fprint(w, cell)
			if i < len(row)-1 {
				fmt.Fprint(w, strings.Repeat(" ", widths[i]-runewidth.StringWidth(cell)))
			}
		}
		fmt.Fprintln(w)
	}
}

func sessionActivityTime(s db.Session) string {
	if s.EndedAt != nil && *s.EndedAt != "" {
		return *s.EndedAt
	}
	if s.StartedAt != nil && *s.StartedAt != "" {
		return *s.StartedAt
	}
	return s.CreatedAt
}

func recentlyActive(ts string, now time.Time) bool {
	t, ok := parseCLITime(ts)
	if !ok {
		return false
	}
	d := now.Sub(t)
	return d >= 0 && d <= resumeActiveWindow
}

func sessionRecentlyActive(s db.Session, now time.Time) bool {
	return recentlyActive(sessionActivityTime(s), now)
}

func humanizeSessionAge(ts string, now time.Time) string {
	t, ok := parseCLITime(ts)
	if !ok {
		return "-"
	}
	return humanizeAge(t, now, false)
}

func humanizeMatchAge(ts string, now time.Time) string {
	t, ok := parseCLITime(ts)
	if !ok {
		return "-"
	}
	return humanizeAge(t, now, true)
}

func humanizeAge(t, now time.Time, includeYear bool) string {
	if t.After(now) {
		return "now"
	}
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", max(0, int(d.Seconds())))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case includeYear && t.Year() != now.Year():
		return t.Format("Jan 2006")
	default:
		return t.Format("Jan 02")
	}
}

func parseCLITime(ts string) (time.Time, bool) {
	if ts == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.000Z", "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, ts); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func sessionDisplayName(s db.Session) string {
	for _, candidate := range []string{ptrString(s.DisplayName), ptrString(s.FirstMessage), s.ID} {
		candidate = collapseWhitespace(candidate)
		if candidate != "" {
			return candidate
		}
	}
	return "-"
}

// collapseHome renders path with the home prefix replaced by "~".
//
// Comparison and output both run on forward slashes rather than the host's
// os.PathSeparator. A session cwd comes out of the archive, which may have been
// recorded on a different machine and OS than the one printing this table — pg
// serve makes that the normal case, not an edge case. Cleaning with the host
// separator rewrote a POSIX cwd into "~\proj" when the table was printed on
// Windows, which is a path that never existed anywhere.
func collapseHome(path, home string) string {
	if path == "" || home == "" {
		return path
	}
	cleanPath := filepath.ToSlash(filepath.Clean(path))
	cleanHome := filepath.ToSlash(filepath.Clean(home))
	if cleanPath == cleanHome {
		return "~"
	}
	if rest, ok := strings.CutPrefix(cleanPath, cleanHome+"/"); ok {
		return "~/" + rest
	}
	return path
}

func truncName(s string, maxCells int) string {
	if maxCells <= 0 || runewidth.StringWidth(s) <= maxCells {
		return s
	}
	if maxCells <= 3 {
		return runewidth.Truncate(s, maxCells, "")
	}
	return runewidth.Truncate(s, maxCells, "...")
}

func collapseWhitespace(s string) string {
	return strings.Join(strings.FieldsFunc(s, unicode.IsSpace), " ")
}

func emptyDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return collapseWhitespace(s)
}

func ptrString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
