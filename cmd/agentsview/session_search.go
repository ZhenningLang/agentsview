// ABOUTME: `session search` subcommand — substring/regex/fts content
// ABOUTME: search across messages and tool I/O with redacted snippets.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/spf13/cobra"
	"go.kenn.io/agentsview/internal/service"
	"golang.org/x/term"
)

func newSessionSearchCommand() *cobra.Command {
	var (
		useRegex, useFTS                  bool
		in                                string
		excludeSystem, reveal             bool
		project, excludeProject, agent    string
		machine, date, dateFrom, dateTo   string
		activeSince                       string
		includeChildren, includeAutomated bool
		includeOneShot                    bool
		limit, cursor                     int
	)
	cmd := &cobra.Command{
		Use:          "search <pattern>",
		Short:        "Search message and tool content across sessions",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if useRegex && useFTS {
				return fmt.Errorf("--regex and --fts are mutually exclusive")
			}
			var sources []string
			for s := range strings.SplitSeq(in, ",") {
				if s = strings.TrimSpace(s); s != "" {
					sources = append(sources, s)
				}
			}
			if useFTS {
				for _, s := range sources {
					if s != "messages" {
						return fmt.Errorf(
							"--fts searches messages only; drop --in or --fts")
					}
				}
			}
			mode := "substring"
			switch {
			case useRegex:
				mode = "regex"
			case useFTS:
				mode = "fts"
			}
			svc, cleanup, err := resolveService(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			res, err := svc.SearchContent(cmd.Context(), service.ContentSearchRequest{
				Pattern:          args[0],
				Mode:             mode,
				Sources:          sources,
				ExcludeSystem:    excludeSystem,
				Reveal:           reveal,
				Project:          project,
				ExcludeProject:   excludeProject,
				Machine:          machine,
				Agent:            agent,
				Date:             date,
				DateFrom:         dateFrom,
				DateTo:           dateTo,
				ActiveSince:      activeSince,
				IncludeChildren:  includeChildren,
				IncludeAutomated: includeAutomated,
				IncludeOneShot:   includeOneShot,
				Limit:            limit,
				Cursor:           cursor,
			})
			if err != nil {
				return err
			}
			if reveal {
				fmt.Fprintln(cmd.ErrOrStderr(),
					"WARNING: --reveal prints full secret values; "+
						"this terminal/session may itself be recorded.")
			}
			if outputFormat(cmd) == "json" {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(res)
			}
			width := 0
			if f, ok := cmd.OutOrStdout().(interface{ Fd() uintptr }); ok && term.IsTerminal(int(f.Fd())) {
				if w, _, err := term.GetSize(int(f.Fd())); err == nil {
					width = w
				}
			}
			return printContentMatchesHumanAt(cmd.OutOrStdout(), res, width, time.Now())
		},
	}
	flags := cmd.Flags()
	flags.BoolVar(&useRegex, "regex", false, "Treat pattern as an RE2 regex")
	flags.BoolVar(&useFTS, "fts", false, "Fast tokenized FTS over messages only")
	flags.StringVar(&in, "in", "",
		"Comma-separated sources: messages,tool_input,tool_result (default all)")
	flags.BoolVar(&excludeSystem, "exclude-system", false,
		"Exclude system messages (included by default)")
	flags.BoolVar(&reveal, "reveal", false, "Show full secret values (unredacted)")
	flags.StringVar(&project, "project", "", "Filter by project name")
	flags.StringVar(&excludeProject, "exclude-project", "", "Exclude project")
	flags.StringVar(&machine, "machine", "", "Filter by machine")
	flags.StringVar(&agent, "agent", "", "Filter by agent")
	flags.StringVar(&date, "date", "", "Sessions started on YYYY-MM-DD")
	flags.StringVar(&dateFrom, "date-from", "", "Sessions on or after YYYY-MM-DD")
	flags.StringVar(&dateTo, "date-to", "", "Sessions on or before YYYY-MM-DD")
	flags.StringVar(&activeSince, "active-since", "", "Active since RFC3339 timestamp")
	flags.BoolVar(&includeChildren, "include-children", false, "Include subagent sessions")
	flags.BoolVar(&includeAutomated, "include-automated", false, "Include automated sessions")
	flags.BoolVar(&includeOneShot, "include-one-shot", false, "Include one-shot sessions")
	flags.IntVar(&limit, "limit", 0, "Max results (default 50, max 500)")
	flags.IntVar(&cursor, "cursor", 0, "Pagination cursor from a previous response")
	return cmd
}

// printContentMatchesHuman writes one line per match, terminal-sanitized.
func printContentMatchesHuman(w io.Writer, res *service.ContentSearchResult) error {
	return printContentMatchesHumanAt(w, res, 0, time.Now())
}

func printContentMatchesHumanAt(w io.Writer, res *service.ContentSearchResult, termWidth int, now time.Time) error {
	if len(res.Matches) == 0 {
		fmt.Fprintln(w, "(no matches)")
		if res.NextCursor != 0 {
			fmt.Fprintf(w, "More results: --cursor %d\n", res.NextCursor)
		}
		return nil
	}
	rows := make([][]string, 0, len(res.Matches)+1)
	rows = append(rows, []string{"ID", "MATCH", "AGE", "PROJECT", "LOCATION", "SNIPPET"})
	for _, m := range res.Matches {
		loc := m.Location
		if m.ToolName != "" {
			loc = m.Location + ":" + m.ToolName
		}
		rows = append(rows, []string{
			sanitizeTerminal(m.SessionID),
			fmt.Sprintf("#%d", m.Ordinal),
			humanizeMatchAge(m.Timestamp, now),
			sanitizeTerminal(collapseWhitespace(m.Project)),
			sanitizeTerminal(collapseWhitespace(loc)),
			sanitizeTerminal(collapseWhitespace(m.Snippet)),
		})
	}
	writeSearchRows(w, rows, termWidth)
	if res.NextCursor != 0 {
		fmt.Fprintf(w, "\nMore results: --cursor %d\n", res.NextCursor)
	}
	return nil
}

func writeSearchRows(w io.Writer, rows [][]string, termWidth int) {
	if termWidth > 0 && termWidth < minSearchTableWidth() {
		writeCompactSearchRows(w, rows, termWidth)
		return
	}
	widths := []int{2, 5, 3, 7, 8, 7}
	for _, row := range rows {
		for i := 0; i < len(row)-1; i++ {
			widths[i] = max(widths[i], runewidth.StringWidth(row[i]))
		}
	}
	if termWidth > 0 {
		projectCap, locationCap := 24, 28
		widths[3] = min(widths[3], projectCap)
		widths[4] = min(widths[4], locationCap)
		shrinkSearchColumns(widths, termWidth)
	}
	for _, row := range rows {
		cells := append([]string(nil), row...)
		if termWidth > 0 {
			for i := 0; i < len(cells)-1; i++ {
				cells[i] = cellTruncate(cells[i], widths[i])
			}
			fixed := widths[0] + widths[1] + widths[2] + widths[3] + widths[4] + 10
			budget := max(0, termWidth-fixed)
			cells[5] = cellTruncate(cells[5], budget)
		}
		for i, cell := range cells {
			if i > 0 {
				fmt.Fprint(w, "  ")
			}
			fmt.Fprint(w, cell)
			if i < len(cells)-1 {
				pad := widths[i] - runewidth.StringWidth(cell)
				if pad > 0 {
					fmt.Fprint(w, strings.Repeat(" ", pad))
				}
			}
		}
		fmt.Fprintln(w)
	}
}

func writeCompactSearchRows(w io.Writer, rows [][]string, termWidth int) {
	for i, row := range rows {
		line := "MATCH SNIPPET"
		if i > 0 {
			line = strings.TrimSpace(row[1] + " " + row[5])
		}
		fmt.Fprintln(w, cellTruncate(line, termWidth))
	}
}

func shrinkSearchColumns(widths []int, termWidth int) {
	minWidths := []int{2, 5, 3, 7, 8}
	for fixedWidth(widths) > termWidth && widths[4] > minWidths[4] {
		widths[4]--
	}
	for fixedWidth(widths) > termWidth && widths[3] > minWidths[3] {
		widths[3]--
	}
	for fixedWidth(widths) > termWidth && widths[1] > minWidths[1] {
		widths[1]--
	}
	for fixedWidth(widths) > termWidth && widths[0] > minWidths[0] {
		widths[0]--
	}
}

func fixedWidth(widths []int) int {
	return widths[0] + widths[1] + widths[2] + widths[3] + widths[4] + 10
}

func minSearchTableWidth() int {
	return fixedWidth([]int{2, 5, 3, 7, 8})
}

func cellTruncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= width {
		return s
	}
	return runewidth.Truncate(s, width, "…")
}
