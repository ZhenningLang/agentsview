package server

import (
	"context"
	"net/http"

	"go.kenn.io/agentsview/internal/db"
)

func (s *Server) registerVaultRoutes() {
	group := newRouteGroup(s.api, "/api/v1/vault", "Vault")

	get(s, group, "/runs", "List dev-workflow runs", s.humaListVaultRuns)
	get(s, group, "/runs/{slug}", "Get one run with phases and metrics", s.humaGetVaultRun)
}

type vaultRunsListInput struct {
	Skill string `query:"skill" doc:"Filter by run skill (e.g. dev-long-run, dev-complete)"`
}

type vaultRunsListOutput struct {
	Runs []db.VaultRun `json:"runs"`
}

func (s *Server) humaListVaultRuns(
	ctx context.Context, in *vaultRunsListInput,
) (*jsonOutput[vaultRunsListOutput], error) {
	runs, err := s.db.ListVaultRuns(ctx, db.VaultFilter{
		Skill: in.Skill,
	})
	if err != nil {
		return nil, apiError(http.StatusInternalServerError, err.Error())
	}
	if runs == nil {
		runs = []db.VaultRun{}
	}
	return &jsonOutput[vaultRunsListOutput]{Body: vaultRunsListOutput{Runs: runs}}, nil
}

type vaultRunGetInput struct {
	// Slug is the run directory name under .long-loop/. Slugs are
	// date-prefixed and contain no slashes, so they ride in one path
	// segment without encoding (unlike memory rel_paths).
	Slug string `path:"slug" doc:"Run slug"`
}

// humaGetVaultRun returns the full run record. The VaultRun already carries
// AcceptanceOK/AcceptanceExit, the phases progress (each with Verify and
// Stuck fields), and the metrics timeline; GetVaultRun attaches phases and
// metrics, so no extra assembly is needed here.
func (s *Server) humaGetVaultRun(
	ctx context.Context, in *vaultRunGetInput,
) (*jsonOutput[db.VaultRun], error) {
	run, err := s.db.GetVaultRun(ctx, in.Slug)
	if err != nil {
		return nil, apiError(http.StatusInternalServerError, err.Error())
	}
	if run == nil {
		return nil, apiError(http.StatusNotFound, "vault run not found")
	}
	if run.Phases == nil {
		run.Phases = []db.VaultPhase{}
	}
	if run.Metrics == nil {
		run.Metrics = []db.VaultMetric{}
	}
	return &jsonOutput[db.VaultRun]{Body: *run}, nil
}
