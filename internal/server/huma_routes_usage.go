package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/service"
)

func (s *Server) registerUsageRoutes() {
	group := newRouteGroup(s.api, "/api/v1/usage", "Usage")

	get(s, group, "/summary", "Get usage summary", s.humaUsageSummary)
	get(s, group, "/comparison", "Get usage comparison", s.humaUsageComparison)
	get(s, group, "/pairwise-comparison", "Get pairwise usage comparison", s.humaUsagePairwiseComparison)
	get(s, group, "/top-sessions", "Get top usage sessions", s.humaUsageTopSessions)
}

type UsageFilterInput struct {
	From             string `query:"from" format:"date" doc:"Range start date"`
	To               string `query:"to" format:"date" doc:"Range end date"`
	Timezone         string `query:"timezone" doc:"IANA timezone name"`
	Agent            string `query:"agent" doc:"Filter by agent"`
	Project          string `query:"project" doc:"Filter by project"`
	Machine          string `query:"machine" doc:"Filter by machine"`
	GitBranch        string `query:"git_branch" doc:"Filter by git branch; opaque (project, branch) tokens from the /branches endpoint"`
	ExcludeProject   string `query:"exclude_project" doc:"Exclude a project"`
	ExcludeAgent     string `query:"exclude_agent" doc:"Exclude an agent"`
	ExcludeModel     string `query:"exclude_model" doc:"Exclude a model"`
	Model            string `query:"model" doc:"Filter by model"`
	MinUserMessages  int    `query:"min_user_messages" minimum:"0" doc:"Minimum user message count"`
	ActiveSince      string `query:"active_since" format:"date-time" doc:"Filter sessions active since this RFC3339 timestamp"`
	IncludeOneShot   bool   `query:"include_one_shot" default:"true" doc:"Include one-shot sessions"`
	IncludeAutomated bool   `query:"include_automated" doc:"Include automated sessions"`
	NoCache          bool   `query:"no_cache" doc:"Bypass the server-side usage cache and recompute"`
}

type usageTopSessionsInput struct {
	UsageFilterInput
	Limit int `query:"limit" minimum:"0" maximum:"100" default:"20" doc:"Maximum number of sessions"`
}

type usageComparisonInput struct {
	UsageFilterInput
	CurrentCost float64 `query:"current_cost" required:"true" doc:"Current period total cost"`
}

type usagePairwiseComparisonInput struct {
	UsageFilterInput
	LeftDimension  string `query:"left_dimension" required:"true" enum:"model,project" doc:"Left comparison dimension"`
	LeftValue      string `query:"left_value" required:"true" doc:"Left comparison value"`
	RightDimension string `query:"right_dimension" required:"true" enum:"model,project" doc:"Right comparison dimension"`
	RightValue     string `query:"right_value" required:"true" doc:"Right comparison value"`
}

// usageFilterFromInput converts the route's query parameters into the
// shared service request and runs the shared defaults and validation, so
// this handler cannot drift from the direct/HTTP service backends. Only
// the error mapping is route-specific.
func usageFilterFromInput(in UsageFilterInput) (db.UsageFilter, error) {
	f, err := service.UsageFilterFromRequest(usageRequestFromInput(in))
	if err != nil {
		var inputErr *service.UsageInputError
		if errors.As(err, &inputErr) {
			return db.UsageFilter{}, apiError(
				http.StatusBadRequest, inputErr.Msg)
		}
		return db.UsageFilter{}, err
	}
	return f, nil
}

// usageRequestFromInput maps query parameters to the service request.
// Huma has already applied the parameter defaults (include_one_shot
// true, include_automated false) by the time the handler runs, so both
// flags are passed as explicit values rather than left nil.
func usageRequestFromInput(in UsageFilterInput) service.UsageRequest {
	return service.UsageRequest{
		From:             in.From,
		To:               in.To,
		Timezone:         in.Timezone,
		Agent:            in.Agent,
		Project:          in.Project,
		Machine:          in.Machine,
		GitBranch:        in.GitBranch,
		ExcludeProject:   in.ExcludeProject,
		ExcludeAgent:     in.ExcludeAgent,
		ExcludeModel:     in.ExcludeModel,
		Model:            in.Model,
		MinUserMessages:  in.MinUserMessages,
		ActiveSince:      in.ActiveSince,
		IncludeOneShot:   &in.IncludeOneShot,
		IncludeAutomated: &in.IncludeAutomated,
		NoCache:          in.NoCache,
	}
}

func (s *Server) humaUsageSummary(
	ctx context.Context,
	in *UsageFilterInput,
) (*jsonOutput[UsageSummaryResponse], error) {
	f, err := usageFilterFromInput(*in)
	if err != nil {
		return nil, err
	}
	result, err := s.cachedDailyUsage(ctx, f, in.NoCache)
	if err != nil {
		if handled := handleHumaContextError(err); handled != nil {
			return nil, handled
		}
		if handled := handleHumaReadOnly(err); handled != nil {
			return nil, handled
		}
		return nil, internalError("usage summary error", err)
	}
	return &jsonOutput[UsageSummaryResponse]{
		Body: *service.BuildUsageSummary(f, result),
	}, nil
}

func (s *Server) humaUsagePairwiseComparison(
	ctx context.Context,
	in *usagePairwiseComparisonInput,
) (*jsonOutput[PairwiseComparisonResponse], error) {
	f, err := usageFilterFromInput(in.UsageFilterInput)
	if err != nil {
		return nil, err
	}
	leftDim, err := parsePairwiseDimension(in.LeftDimension)
	if err != nil {
		return nil, err
	}
	rightDim, err := parsePairwiseDimension(in.RightDimension)
	if err != nil {
		return nil, err
	}
	f.Breakdowns = false
	leftFilter, leftEmpty := pairwiseFilter(f, leftDim, in.LeftValue)
	rightFilter, rightEmpty := pairwiseFilter(f, rightDim, in.RightValue)
	leftResult, err := s.pairwiseDailyUsage(ctx, leftFilter, leftEmpty, in.NoCache)
	if err != nil {
		if handled := handleHumaContextError(err); handled != nil {
			return nil, handled
		}
		if handled := handleHumaReadOnly(err); handled != nil {
			return nil, handled
		}
		return nil, internalError("usage pairwise left error", err)
	}
	rightResult, err := s.pairwiseDailyUsage(ctx, rightFilter, rightEmpty, in.NoCache)
	if err != nil {
		if handled := handleHumaContextError(err); handled != nil {
			return nil, handled
		}
		if handled := handleHumaReadOnly(err); handled != nil {
			return nil, handled
		}
		return nil, internalError("usage pairwise right error", err)
	}
	resp := service.BuildPairwiseComparison(
		PairwiseSide{Dimension: leftDim, Value: in.LeftValue, Empty: leftEmpty}, leftResult,
		PairwiseSide{Dimension: rightDim, Value: in.RightValue, Empty: rightEmpty}, rightResult,
	)
	return &jsonOutput[PairwiseComparisonResponse]{Body: resp}, nil
}

func parsePairwiseDimension(raw string) (PairwiseDimension, error) {
	dim, err := service.ParsePairwiseDimension(raw)
	if err != nil {
		var inputErr *service.UsageInputError
		if errors.As(err, &inputErr) {
			return "", apiError(http.StatusBadRequest, inputErr.Msg)
		}
		return "", err
	}
	return dim, nil
}

func (s *Server) pairwiseDailyUsage(
	ctx context.Context, f db.UsageFilter, empty bool, noCache bool,
) (db.DailyUsageResult, error) {
	if empty {
		return service.EmptyUsageResult(), nil
	}
	return s.cachedDailyUsage(ctx, f, noCache)
}

func pairwiseFilter(
	f db.UsageFilter, dim PairwiseDimension, value string,
) (db.UsageFilter, bool) {
	return service.PairwiseFilter(f, dim, value)
}

func (s *Server) humaUsageComparison(
	ctx context.Context,
	in *usageComparisonInput,
) (*jsonOutput[Comparison], error) {
	f, err := usageFilterFromInput(in.UsageFilterInput)
	if err != nil {
		return nil, err
	}
	comparison, err := s.computeUsageComparison(ctx, f, in.CurrentCost, in.NoCache)
	if err != nil {
		if handled := handleHumaContextError(err); handled != nil {
			return nil, handled
		}
		if handled := handleHumaReadOnly(err); handled != nil {
			return nil, handled
		}
		return nil, internalError("usage comparison error", err)
	}
	return &jsonOutput[Comparison]{Body: *comparison}, nil
}

func (s *Server) computeUsageComparison(
	ctx context.Context,
	f db.UsageFilter,
	currentCost float64,
	noCache bool,
) (*Comparison, error) {
	fromT, err := time.Parse("2006-01-02", f.From)
	if err != nil {
		return nil, err
	}
	toT, err := time.Parse("2006-01-02", f.To)
	if err != nil {
		return nil, err
	}
	days := int(toT.Sub(fromT).Hours()/24) + 1
	priorTo := fromT.AddDate(0, 0, -1)
	priorFrom := priorTo.AddDate(0, 0, -(days - 1))
	priorFilter := db.UsageFilter{
		From:             priorFrom.Format("2006-01-02"),
		To:               priorTo.Format("2006-01-02"),
		Agent:            f.Agent,
		Project:          f.Project,
		Machine:          f.Machine,
		GitBranch:        f.GitBranch,
		Model:            f.Model,
		ExcludeProject:   f.ExcludeProject,
		ExcludeAgent:     f.ExcludeAgent,
		ExcludeModel:     f.ExcludeModel,
		Timezone:         f.Timezone,
		MinUserMessages:  f.MinUserMessages,
		ExcludeOneShot:   f.ExcludeOneShot,
		ExcludeAutomated: f.ExcludeAutomated,
		ActiveSince:      f.ActiveSince,
		Breakdowns:       false,
	}
	priorResult, err := s.cachedDailyUsage(ctx, priorFilter, noCache)
	if err != nil {
		return nil, err
	}
	c := &Comparison{
		PriorFrom:      priorFilter.From,
		PriorTo:        priorFilter.To,
		PriorTotalCost: priorResult.Totals.TotalCost,
	}
	if c.PriorTotalCost > 0 {
		c.DeltaPct = (currentCost - c.PriorTotalCost) / c.PriorTotalCost
	}
	return c, nil
}

func (s *Server) humaUsageTopSessions(
	ctx context.Context,
	in *usageTopSessionsInput,
) (*jsonOutput[[]db.TopSessionEntry], error) {
	f, err := usageFilterFromInput(in.UsageFilterInput)
	if err != nil {
		return nil, err
	}
	f.Breakdowns = false
	limit := in.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	entries, err := s.cachedTopSessions(ctx, f, limit, in.NoCache)
	if err != nil {
		if handled := handleHumaContextError(err); handled != nil {
			return nil, handled
		}
		if handled := handleHumaReadOnly(err); handled != nil {
			return nil, handled
		}
		return nil, internalError("usage top sessions error", err)
	}
	return &jsonOutput[[]db.TopSessionEntry]{Body: entries}, nil
}
