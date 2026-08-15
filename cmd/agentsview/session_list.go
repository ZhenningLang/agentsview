// ABOUTME: `session list` subcommand — lists sessions with the
// ABOUTME: full set of HTTP query-param equivalents as CLI flags.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/service"
)

func newSessionListCommand() *cobra.Command {
	var (
		project, excludeProject, machine, agent string
		date, dateFrom, dateTo, activeSince     string
		minMessages, maxMessages                int
		minUserMessages                         int
		includeOneShot                          bool
		includeAutomated, includeChildren       bool
		outcome, healthGrade                    string
		minToolFailures                         int
		hasSecret, resume, active               bool
		cursor                                  string
		limit                                   int
		sort                                    string
		reverse                                 bool
	)
	cmd := &cobra.Command{
		Use:          "list",
		Short:        "List sessions with filters",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			keys, err := db.ParseSortSpec(sort)
			if err != nil {
				return fmt.Errorf("invalid sort %q: %w", sort, err)
			}
			if len(keys) == 0 {
				keys = []db.SortKey{{Key: db.DefaultSortKey()}}
			}
			if reverse {
				for i := range keys {
					if keys[i].Descending == nil {
						d := !db.SortDefaultDescending(keys[i].Key)
						keys[i].Descending = &d
					}
				}
			}

			svc, cleanup, err := resolveService(cmd)
			if err != nil {
				return err
			}
			defer cleanup()

			if (resume || active) && !cmd.Flags().Changed("active-since") {
				activeSince = time.Now().UTC().Add(-resumeActiveWindow).Format(time.RFC3339)
			}
			f := service.ListFilter{
				Project:          project,
				ExcludeProject:   excludeProject,
				Machine:          machine,
				Agent:            agent,
				Date:             date,
				DateFrom:         dateFrom,
				DateTo:           dateTo,
				ActiveSince:      activeSince,
				MinMessages:      minMessages,
				MaxMessages:      maxMessages,
				MinUserMessages:  minUserMessages,
				IncludeOneShot:   includeOneShot,
				IncludeAutomated: includeAutomated,
				IncludeChildren:  includeChildren,
				Outcome:          outcome,
				HealthGrade:      healthGrade,
				HasSecret:        hasSecret,
				Cursor:           cursor,
				Limit:            limit,
				OrderBy:          db.FormatSortSpec(keys),
			}
			if cmd.Flags().Changed("min-tool-failures") {
				f.MinToolFailures = &minToolFailures
			}

			list, err := svc.List(cmd.Context(), f)
			if err != nil {
				return err
			}
			if outputFormat(cmd) == "json" {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(list)
			}
			home, _ := os.UserHomeDir()
			return printSessionListHumanAt(cmd.OutOrStdout(), list, time.Now(), home)
		},
	}

	flags := cmd.Flags()
	flags.StringVar(&project, "project", "",
		"Filter by project name")
	flags.StringVar(&excludeProject, "exclude-project", "",
		"Exclude sessions from the given project")
	flags.StringVar(&machine, "machine", "",
		"Filter by machine name")
	flags.StringVar(&agent, "agent", "",
		"Filter by agent (claude, codex, cursor, ...)")
	flags.StringVar(&date, "date", "",
		"Filter sessions started on YYYY-MM-DD")
	flags.StringVar(&dateFrom, "date-from", "",
		"Filter sessions started on or after YYYY-MM-DD")
	flags.StringVar(&dateTo, "date-to", "",
		"Filter sessions started on or before YYYY-MM-DD")
	flags.StringVar(&activeSince, "active-since", "",
		"Filter sessions active since RFC3339 timestamp")
	flags.IntVar(&minMessages, "min-messages", 0,
		"Minimum total message count")
	flags.IntVar(&maxMessages, "max-messages", 0,
		"Maximum total message count")
	flags.IntVar(&minUserMessages, "min-user-messages", 0,
		"Minimum user message count")
	flags.BoolVar(&includeOneShot, "include-one-shot", false,
		"Include one-shot sessions (excluded by default)")
	flags.BoolVar(&includeAutomated, "include-automated", false,
		"Include automated sessions (excluded by default)")
	flags.BoolVar(&includeChildren, "include-children", false,
		"Include subagent/child sessions")
	flags.StringVar(&outcome, "outcome", "",
		"Filter by outcome (comma-separated: success,failure,...)")
	flags.StringVar(&healthGrade, "health-grade", "",
		"Filter by health grade (comma-separated: A,B,C,D,F)")
	flags.IntVar(&minToolFailures, "min-tool-failures", 0,
		"Minimum tool-failure signal count (0 is a valid filter)")
	flags.BoolVar(&hasSecret, "has-secret", false,
		"Only sessions with detected secret leaks")
	flags.BoolVar(&resume, "resume", false,
		"Show recently active sessions from the last 15 minutes")
	flags.BoolVar(&active, "active", false,
		"Alias for --resume")
	flags.StringVar(&cursor, "cursor", "",
		"Pagination cursor from a previous response")
	flags.IntVar(&limit, "limit", 0,
		fmt.Sprintf(
			"Maximum sessions to return (default %d, max %d)",
			db.DefaultSessionLimit, db.MaxSessionLimit,
		))
	flags.StringVar(&sort, "sort", "recent",
		"Sort by comma-separated keys, each optionally key:asc or key:desc. Keys: "+
			strings.Join(db.SortKeys(), ", "))
	flags.BoolVarP(&reverse, "reverse", "r", false,
		"Reverse sort keys that have no explicit :asc/:desc suffix")
	return cmd
}
