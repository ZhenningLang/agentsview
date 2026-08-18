package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	stdsync "sync"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/insight"
	"go.kenn.io/agentsview/internal/timeutil"
)

func (s *Server) registerInsightsRoutes() {
	group := newRouteGroup(s.api, "/api/v1/insights", "Insights")

	get(s, group, "", "List insights", s.humaListInsights)
	get(s, group, "/{id}", "Get insight", s.humaGetInsight)
	raw(s, group, http.MethodGet, "/{id}/export",
		"Export insight as HTML", s.humaExportInsight)
	raw(s, group, http.MethodGet, "/{id}/md",
		"Export insight as Markdown", s.humaMarkdownInsight)
	post(s, group, "/{id}/publish", "Publish insight", s.humaPublishInsight)
	deleteRoute(s, group, "/{id}", "Delete insight", s.humaDeleteInsight)
	stream(s, group, http.MethodPost, "/generate", "Generate insight", s.humaGenerateInsight)
}

type insightType string

type insightsInput struct {
	Type    insightType `query:"type" enum:"daily_activity,agent_analysis" doc:"Insight type"`
	Project string      `query:"project" doc:"Filter by project"`
}

type insightsResponse struct {
	Insights []db.Insight `json:"insights"`
}

type generateInsightInput struct {
	Body generateInsightRequest
}

type publishInsightInput struct {
	ID     int64 `path:"id" required:"true" doc:"Insight ID"`
	Secret bool  `query:"secret" doc:"Create a secret gist instead of a public one"`
}

func (s *Server) humaListInsights(
	ctx context.Context,
	in *insightsInput,
) (*jsonOutput[insightsResponse], error) {
	insights, err := s.db.ListInsights(ctx, db.InsightFilter{
		Type:    string(in.Type),
		Project: in.Project,
	})
	if err != nil {
		return nil, serverError(err)
	}
	if insights == nil {
		insights = []db.Insight{}
	}
	return &jsonOutput[insightsResponse]{
		Body: insightsResponse{Insights: insights},
	}, nil
}

// insightByID is the single lookup seam for every per-insight route.
// Centralizing it keeps the DB-error and 404 mapping identical across
// get, delete and the export routes instead of drifting per handler.
func (s *Server) insightByID(
	ctx context.Context,
	id int64,
) (*db.Insight, error) {
	result, err := s.db.GetInsight(ctx, id)
	if err != nil {
		return nil, serverError(err)
	}
	if result == nil {
		return nil, apiError(http.StatusNotFound, "insight not found")
	}
	return result, nil
}

func (s *Server) humaGetInsight(
	ctx context.Context,
	in *intIDPathInput,
) (*jsonOutput[*db.Insight], error) {
	result, err := s.insightByID(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	return &jsonOutput[*db.Insight]{Body: result}, nil
}

func (s *Server) humaExportInsight(
	ctx context.Context,
	in *intIDPathInput,
) (*bytesOutput, error) {
	item, err := s.insightByID(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	return &bytesOutput{
		ContentType: "text/html; charset=utf-8",
		ContentDisposition: fmt.Sprintf(`attachment; filename="%s"`,
			insightExportHTMLFilename(item)),
		Body: []byte(generateInsightExportHTML(item)),
	}, nil
}

func (s *Server) humaMarkdownInsight(
	ctx context.Context,
	in *intIDPathInput,
) (*bytesOutput, error) {
	item, err := s.insightByID(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	// Served verbatim: the stored content is already Markdown, so the
	// route must not re-render or escape it.
	return &bytesOutput{
		ContentType: "text/markdown; charset=utf-8",
		ContentDisposition: fmt.Sprintf(`inline; filename="%s"`,
			insightExportMarkdownFilename(item)),
		Body: []byte(item.Content),
	}, nil
}

// humaPublishInsight uploads the insight's HTML export as a gist. It goes
// through publishExportHTMLWithURL with s.gistAPIEndpoint() rather than the
// hardcoded createGist, so the destination is a single injectable seam.
func (s *Server) humaPublishInsight(
	ctx context.Context,
	in *publishInsightInput,
) (*jsonOutput[publishResponse], error) {
	token := s.githubToken()
	if token == "" {
		return nil, apiError(http.StatusUnauthorized,
			"GitHub token not configured")
	}
	item, err := s.insightByID(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	// Secret is the caller's opt-in to a private gist, so it inverts
	// into GitHub's "public" flag exactly once, here.
	resp, err := publishExportHTMLWithURL(
		ctx,
		s.gistAPIEndpoint(),
		token,
		insightExportHTMLFilename(item),
		insightPublishDescription(item),
		generateInsightExportHTML(item),
		!in.Secret,
	)
	if err != nil {
		return nil, apiError(http.StatusBadGateway, err.Error())
	}
	return &jsonOutput[publishResponse]{Body: *resp}, nil
}

func (s *Server) humaDeleteInsight(
	ctx context.Context,
	in *intIDPathInput,
) (*noContentOutput, error) {
	if _, err := s.insightByID(ctx, in.ID); err != nil {
		return nil, err
	}
	if err := s.db.DeleteInsight(in.ID); err != nil {
		if handled := handleHumaReadOnly(err); handled != nil {
			return nil, handled
		}
		return nil, serverError(err)
	}
	return &noContentOutput{Status: http.StatusNoContent}, nil
}

func (s *Server) humaGenerateInsight(
	ctx context.Context,
	in *generateInsightInput,
) (*huma.StreamResponse, error) {
	if s.db.ReadOnly() {
		return nil, apiError(http.StatusNotImplemented,
			"insight generation is not available in read-only mode")
	}
	req := in.Body
	if !validInsightTypes[req.Type] {
		return nil, apiError(http.StatusBadRequest,
			"invalid type: must be daily_activity or agent_analysis")
	}
	if !timeutil.IsValidDate(req.DateFrom) {
		return nil, apiError(http.StatusBadRequest,
			"invalid date_from: use YYYY-MM-DD")
	}
	if !timeutil.IsValidDate(req.DateTo) {
		return nil, apiError(http.StatusBadRequest,
			"invalid date_to: use YYYY-MM-DD")
	}
	if req.DateTo < req.DateFrom {
		return nil, apiError(http.StatusBadRequest,
			"date_to must be >= date_from")
	}
	if req.Agent == "" {
		req.Agent = "claude"
	}
	if !insight.ValidAgents[req.Agent] {
		return nil, apiError(http.StatusBadRequest,
			"invalid agent: must be one of "+
				strings.Join(insight.ValidAgentNames, ", "))
	}
	return &huma.StreamResponse{Body: func(hctx huma.Context) {
		stream, ok := newHumaSSEStream(hctx)
		if !ok {
			writeHumaJSON(hctx, http.StatusInternalServerError,
				apiErrorResponse{Message: "streaming not supported"})
			return
		}
		var streamMu stdsync.Mutex
		sendJSON := func(event string, v any) bool {
			streamMu.Lock()
			defer streamMu.Unlock()
			return stream.SendJSON(event, v)
		}
		if !sendJSON("status", map[string]string{"phase": "generating"}) {
			return
		}
		prompt, err := insight.BuildPrompt(hctx.Context(), s.db, insight.GenerateRequest{
			Type:     req.Type,
			DateFrom: req.DateFrom,
			DateTo:   req.DateTo,
			Project:  req.Project,
			Prompt:   req.Prompt,
		})
		if err != nil {
			log.Printf("insight prompt error: %v", err)
			sendJSON("error", map[string]string{"message": "failed to build prompt"})
			return
		}
		genCtx, cancel := context.WithTimeout(hctx.Context(), 10*time.Minute)
		defer cancel()

		const (
			maxBufferedLogEvents = 256
			logDrainTimeout      = 2 * time.Second
			logStopWaitTimeout   = 500 * time.Millisecond
		)
		logCh := make(chan insight.LogEvent, maxBufferedLogEvents)
		logDone := make(chan struct{})
		logStop := make(chan struct{})
		var logStopOnce stdsync.Once
		stopLogSender := func() {
			logStopOnce.Do(func() { close(logStop) })
		}
		go func() {
			defer close(logDone)
			for {
				select {
				case <-logStop:
					return
				default:
				}
				select {
				case <-logStop:
					return
				case ev, ok := <-logCh:
					if !ok {
						return
					}
					if !sendJSON("log", ev) {
						stopLogSender()
						return
					}
				}
			}
		}()
		var (
			logStateMu    stdsync.Mutex
			logStreamDone bool
			droppedLogs   int
		)
		enqueueLog := func(ev insight.LogEvent) {
			logStateMu.Lock()
			defer logStateMu.Unlock()
			if logStreamDone {
				return
			}
			select {
			case logCh <- ev:
			default:
				droppedLogs++
			}
		}
		finishLogStream := func() (dropped int, drained bool, senderStopped bool, timedOut bool) {
			logStateMu.Lock()
			logStreamDone = true
			close(logCh)
			dropped = droppedLogs
			logStateMu.Unlock()
			select {
			case <-logDone:
				return dropped, true, true, false
			case <-time.After(logDrainTimeout):
				log.Printf("insight log stream drain timed out after %s", logDrainTimeout)
				dropped += len(logCh)
				stopLogSender()
				select {
				case <-logDone:
					return dropped, false, true, true
				case <-time.After(logStopWaitTimeout):
					log.Printf("insight log sender stop timed out after %s", logStopWaitTimeout)
					stream.ForceWriteDeadlineNow()
					select {
					case <-logDone:
						return dropped, false, true, true
					case <-time.After(logStopWaitTimeout):
						log.Printf("insight log sender did not stop after forced deadline")
						return dropped, false, false, true
					}
				}
			}
		}

		result, err := s.generateStreamFunc(genCtx, req.Agent, prompt, enqueueLog)
		dropped, drained, senderStopped, timedOut := finishLogStream()
		if !senderStopped {
			stream.ForceWriteDeadlineNow()
			log.Printf("insight log stream sender did not stop; aborting terminal SSE events")
			return
		}
		if dropped > 0 {
			suffix := "due to slow client"
			if timedOut {
				suffix = "due to slow client and log stream timeout"
			}
			sendJSON("log", insight.LogEvent{
				Stream: "stderr",
				Line:   fmt.Sprintf("dropped %d log line(s) %s", dropped, suffix),
			})
		}
		if timedOut || !drained {
			log.Printf("insight log stream did not fully drain before completion")
			sendJSON("error", map[string]string{
				"message": "insight log stream timed out before completion",
			})
			return
		}
		if err != nil {
			log.Printf("insight generate error: %v", err)
			sendJSON("error", map[string]string{
				"message": insightGenerateClientMessage(req.Agent, err),
			})
			return
		}
		if strings.TrimSpace(result.Content) == "" {
			sendJSON("error", map[string]string{
				"message": "agent returned empty content",
			})
			return
		}
		var project *string
		if req.Project != "" {
			project = &req.Project
		}
		var model *string
		if result.Model != "" {
			model = &result.Model
		}
		var promptPtr *string
		if req.Prompt != "" {
			promptPtr = &req.Prompt
		}
		id, err := s.db.InsertInsight(db.Insight{
			Type:     req.Type,
			DateFrom: req.DateFrom,
			DateTo:   req.DateTo,
			Project:  project,
			Agent:    result.Agent,
			Model:    model,
			Prompt:   promptPtr,
			Content:  result.Content,
		})
		if err != nil {
			log.Printf("insight insert error: %v", err)
			sendJSON("error", map[string]string{"message": "failed to save insight"})
			return
		}
		saved, err := s.db.GetInsight(hctx.Context(), id)
		if err != nil || saved == nil {
			log.Printf("insight get error: id=%d err=%v", id, err)
			sendJSON("error", map[string]string{
				"message": "failed to retrieve saved insight",
			})
			return
		}
		sendJSON("done", saved)
	}}, nil
}
