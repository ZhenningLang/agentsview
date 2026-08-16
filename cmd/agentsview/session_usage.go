// ABOUTME: `session usage <id>` subcommand — prints per-session
// ABOUTME: token statistics and a cost estimate (JSON or human).
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.kenn.io/agentsview/internal/db"
)

func newSessionUsageCommand() *cobra.Command {
	return &cobra.Command{
		Use:          "usage <id>",
		Short:        "Show token usage and cost estimate for a session",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		Run: func(cmd *cobra.Command, args []string) {
			if remote, _ := cmd.Flags().GetString("server"); remote != "" {
				code := runRemoteSessionUsage(cmd, remote, args[0], outputFormat(cmd))
				os.Exit(code)
			}
			runSessionUsage(args[0], outputFormat(cmd))
		},
	}
}

func runRemoteSessionUsage(cmd *cobra.Command, remote, sessionID, format string) int {
	return runRemoteSessionUsageToWriters(cmd, remote, sessionID, format,
		cmd.OutOrStdout(), cmd.ErrOrStderr())
}

func runRemoteSessionUsageToWriters(cmd *cobra.Command, remote, sessionID, format string, stdout, stderr io.Writer) int {
	token, err := explicitServerToken(cmd)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return tokenUseExitErr
	}
	svc, cleanup, err := resolveService(cmd)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return tokenUseExitErr
	}
	defer cleanup()
	resolved, err := resolveServiceSessionID(cmd.Context(), svc, sessionID)
	if err != nil {
		fmt.Fprintf(stderr, "%v\n", err)
		if !strings.HasPrefix(err.Error(), "session not found:") {
			return tokenUseExitErr
		}
		return tokenUseExitNotFound
	}
	out, code, err := remoteSessionUsageData(cmd.Context(), remote, token, resolved)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return tokenUseExitErr
	}
	if out != nil {
		if format == "json" {
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(out); err != nil {
				fmt.Fprintf(stderr, "error: %v\n", err)
				return tokenUseExitErr
			}
		} else if err := renderSessionUsageHuman(stdout, out); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return tokenUseExitErr
		}
	}
	return code
}

func remoteSessionUsageData(ctx context.Context, remote, token, id string) (*sessionUsageOutput, int, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		strings.TrimSuffix(remote, "/")+"/api/v1/sessions/"+url.PathEscape(id)+"/usage?breakdown=true&rollup=true", nil)
	if err != nil {
		return nil, tokenUseExitErr, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, tokenUseExitErr, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, tokenUseExitNotFound, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, tokenUseExitErr, fmt.Errorf("session usage: HTTP %d", resp.StatusCode)
	}
	var usage db.SessionUsage
	if err := json.NewDecoder(resp.Body).Decode(&usage); err != nil {
		return nil, tokenUseExitErr, err
	}
	return &sessionUsageOutput{SessionUsage: usage, ServerRunning: true}, usageExitCode(&usage), nil
}

// runSessionUsage computes usage for one session and renders it,
// exiting with the shared usage exit code (0 = token data or cost,
// 2 = not found, 3 = neither). Uses Run + os.Exit (not RunE) so the
// 2/3 codes survive — cobra RunE errors collapse to exit 1.
func runSessionUsage(sessionID, format string) {
	out, code, err := sessionUsageData(sessionID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(tokenUseExitErr)
	}
	if out != nil {
		if format == "json" {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			if encErr := enc.Encode(out); encErr != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", encErr)
				os.Exit(tokenUseExitErr)
			}
		} else if rerr := renderSessionUsageHuman(
			os.Stdout, out,
		); rerr != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", rerr)
			os.Exit(tokenUseExitErr)
		}
	}
	os.Exit(code)
}

// renderSessionUsageHuman writes a compact key/value summary. The
// cost line shows "~$X.XX (models)" when a complete estimate exists,
// otherwise "n/a" (noting any unpriced models). The tilde marks the
// figure as a model-pricing estimate.
func renderSessionUsageHuman(w io.Writer, out *sessionUsageOutput) error {
	label := func(name string) string {
		return fmt.Sprintf("%-14s", name+":")
	}
	fmt.Fprintf(w, "%s %s\n", label("Session"),
		sanitizeTerminal(out.SessionID))
	fmt.Fprintf(w, "%s %s\n", label("Agent"),
		sanitizeTerminal(out.Agent))
	fmt.Fprintf(w, "%s %d\n", label("Output"), out.TotalOutputTokens)
	fmt.Fprintf(w, "%s %d\n", label("Peak ctx"), out.PeakContextTokens)
	if out.HasCost {
		models := strings.Join(out.Models, ", ")
		fmt.Fprintf(w, "%s ~$%.2f (%s)\n", label("Cost"),
			out.CostUSD, sanitizeTerminal(models))
	} else if len(out.UnpricedModels) > 0 {
		fmt.Fprintf(w, "%s n/a (unpriced: %s)\n", label("Cost"),
			sanitizeTerminal(strings.Join(out.UnpricedModels, ", ")))
	} else {
		fmt.Fprintf(w, "%s n/a\n", label("Cost"))
	}
	if out.RollupSubagentCount > 0 {
		if out.HasRollupCost {
			fmt.Fprintf(w, "%s ~$%.2f (%d subagents)\n",
				label("Total cost"), out.RollupCostUSD,
				out.RollupSubagentCount)
		} else {
			fmt.Fprintf(w, "%s n/a (%d subagents; incomplete pricing)\n",
				label("Total cost"), out.RollupSubagentCount)
		}
	}
	if out.BreakdownCount > 0 {
		fmt.Fprintf(w, "%s %d rows\n", label("Breakdown"), out.BreakdownCount)
	}
	return nil
}
