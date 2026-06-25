package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"go.kenn.io/agentsview/internal/config"
	"go.kenn.io/agentsview/internal/enrich"
	"go.kenn.io/agentsview/internal/llm"
)

type enrichCLIOptions struct {
	All     bool
	Project string
	Force   bool
	Limit   int
}

func newEnrichCommand() *cobra.Command {
	var opts enrichCLIOptions
	cmd := &cobra.Command{
		Use:          "enrich",
		Short:        "Run offline LLM enrichment for local sessions",
		GroupID:      groupData,
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if opts.Limit < 0 {
				return fmt.Errorf("--limit must be >= 0")
			}
			if opts.All && opts.Limit > 0 {
				return fmt.Errorf("--all cannot be combined with --limit")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := mustLoadConfig(cmd)
			return runEnrich(cmd, cfg, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.All, "all", false, "Process all matching sessions without a limit")
	cmd.Flags().StringVar(&opts.Project, "project", "", "Only enrich sessions in this project")
	cmd.Flags().BoolVar(&opts.Force, "force", false, "Bypass status, incremental, and idle gates")
	cmd.Flags().IntVar(&opts.Limit, "limit", 0, "Maximum number of matching sessions to enrich")
	return cmd
}

func runEnrich(cmd *cobra.Command, cfg config.Config, opts enrichCLIOptions) error {
	llmCfg := resolveEnrichLLM(cfg)
	if !llmCfg.Enabled {
		return fmt.Errorf("LLM enrichment is disabled; set [llm].enabled=true and configure an API key before running enrich")
	}
	database, err := openDB(cfg)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer database.Close()
	runner := enrich.New(database, llm.New(llmCfg), llmCfg)
	stats, err := runner.Run(cmd.Context(), enrich.Options{
		Project: opts.Project,
		Force:   opts.Force,
		Limit:   opts.Limit,
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(cmd.OutOrStdout(), "LLM enrichment complete: candidates=%d succeeded=%d failed=%d no_content=%d skipped_too_short=%d\n",
		stats.Candidates, stats.Succeeded, stats.Failed, stats.NoContent, stats.SkippedTooShort)
	return err
}

func resolveEnrichLLM(cfg config.Config) config.LLMConfig {
	return cfg.ResolveUsageLLM("enrich")
}
