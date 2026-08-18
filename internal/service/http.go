// ABOUTME: httpBackend implements SessionService by proxying HTTP
// ABOUTME: calls to a running agentsview daemon.
package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/db"
)

// errHTTPNotFound is returned by getJSON for 404 responses so callers
// can distinguish "no such resource" from other transport errors
// without string-matching the status code. Kept unexported since
// only Get currently consumes it; other paths map status codes
// explicitly below.
var errHTTPNotFound = errors.New("http: not found")

// httpStatusError carries the status code of a non-OK daemon response so
// callers can map a specific code to a transport-neutral sentinel without
// string-matching. Its message is the same one getJSON used to build, so
// callers that only print the error are unaffected.
type httpStatusError struct {
	Path   string
	Status int
	Body   string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("GET %s: HTTP %d: %s", e.Path, e.Status, e.Body)
}

type httpBackend struct {
	baseURL  string
	client   *http.Client
	readOnly bool
	token    string
}

// NewHTTPBackend constructs a SessionService that proxies to a
// running agentsview daemon at baseURL. When readOnly is true,
// Sync returns a clear error without making the HTTP round-trip.
// token, when non-empty, is attached as `Authorization: Bearer ...`
// on every request so the backend works against daemons running
// with require_auth=true.
func NewHTTPBackend(baseURL, token string, readOnly bool) SessionService {
	return &httpBackend{
		baseURL:  strings.TrimSuffix(baseURL, "/"),
		client:   &http.Client{Timeout: 30 * time.Second},
		readOnly: readOnly,
		token:    token,
	}
}

func (b *httpBackend) Get(
	ctx context.Context, id string,
) (*SessionDetail, error) {
	var out SessionDetail
	path := "/api/v1/sessions/" + url.PathEscape(id)
	err := b.getJSON(ctx, path, &out)
	if errors.Is(err, errHTTPNotFound) {
		// Match directBackend.Get: absent session returns (nil, nil)
		// so transport swaps stay neutral.
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}

func (b *httpBackend) List(
	ctx context.Context, f ListFilter,
) (*SessionList, error) {
	q := filterToQuery(f)
	var out SessionList
	if err := b.getJSON(ctx, "/api/v1/sessions?"+q.Encode(), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// filterToQuery converts a ListFilter into the URL query params
// expected by handleListSessions. Field mapping mirrors the
// server-side parser in internal/server/sessions.go.
func filterToQuery(f ListFilter) url.Values {
	q := url.Values{}
	setIfNotEmpty := func(k, v string) {
		if v != "" {
			q.Set(k, v)
		}
	}
	setIfNotEmpty("project", f.Project)
	setIfNotEmpty("exclude_project", f.ExcludeProject)
	setIfNotEmpty("machine", f.Machine)
	setIfNotEmpty("git_branch", f.GitBranch)
	setIfNotEmpty("agent", f.Agent)
	setIfNotEmpty("date", f.Date)
	setIfNotEmpty("date_from", f.DateFrom)
	setIfNotEmpty("date_to", f.DateTo)
	setIfNotEmpty("active_since", f.ActiveSince)
	if f.MinMessages > 0 {
		q.Set("min_messages", strconv.Itoa(f.MinMessages))
	}
	if f.MaxMessages > 0 {
		q.Set("max_messages", strconv.Itoa(f.MaxMessages))
	}
	if f.MinUserMessages > 0 {
		q.Set("min_user_messages", strconv.Itoa(f.MinUserMessages))
	}
	if f.IncludeOneShot {
		q.Set("include_one_shot", "true")
	}
	if f.IncludeAutomated {
		q.Set("include_automated", "true")
	}
	if f.IncludeChildren {
		q.Set("include_children", "true")
	}
	setIfNotEmpty("outcome", f.Outcome)
	setIfNotEmpty("health_grade", f.HealthGrade)
	setIfNotEmpty("termination", f.Termination)
	if f.MinToolFailures != nil {
		q.Set("min_tool_failures", strconv.Itoa(*f.MinToolFailures))
	}
	if f.HasSecret {
		q.Set("has_secret", "true")
	}
	setIfNotEmpty("cursor", f.Cursor)
	if f.Limit > 0 {
		q.Set("limit", strconv.Itoa(f.Limit))
	}
	setIfNotEmpty("order_by", f.OrderBy)
	if f.Descending != nil {
		q.Set("descending", strconv.FormatBool(*f.Descending))
	}
	return q
}

func (b *httpBackend) Messages(
	ctx context.Context, id string, f MessageFilter,
) (*MessageList, error) {
	q := url.Values{}
	if f.From != nil {
		q.Set("from", strconv.Itoa(*f.From))
	}
	if f.Limit > 0 {
		q.Set("limit", strconv.Itoa(f.Limit))
	}
	if f.Direction != "" {
		q.Set("direction", f.Direction)
	}
	path := "/api/v1/sessions/" + url.PathEscape(id) +
		"/messages?" + q.Encode()
	var out MessageList
	if err := b.getJSON(ctx, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (b *httpBackend) ToolCalls(
	ctx context.Context, id string,
) (*ToolCallList, error) {
	var out ToolCallList
	path := "/api/v1/sessions/" + url.PathEscape(id) + "/tool-calls"
	if err := b.getJSON(ctx, path, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (b *httpBackend) Sync(
	ctx context.Context, in SyncInput,
) (*SessionDetail, error) {
	if b.readOnly {
		// Return the shared sentinel so callers can
		// errors.Is(err, db.ErrReadOnly) regardless of
		// transport.
		return nil, fmt.Errorf(
			"sync: daemon at %s is read-only: %w",
			b.baseURL, db.ErrReadOnly,
		)
	}
	body, err := json.Marshal(in)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost,
		b.baseURL+"/api/v1/sessions/sync",
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	// The daemon's CSRF guard rejects mutating requests whose Origin
	// is not in the allowlist. Setting Origin to the daemon's own
	// baseURL satisfies that check for the CLI, which has no real
	// browser origin.
	req.Header.Set("Origin", b.baseURL)
	b.addAuth(req)
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotImplemented {
		// Daemon is read-only (pg serve). Surface as the shared
		// sentinel so CLI callers can errors.Is it.
		return nil, fmt.Errorf(
			"sync: daemon at %s: %w", b.baseURL, db.ErrReadOnly,
		)
	}
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf(
			"sync: HTTP %d: %s", resp.StatusCode, msg,
		)
	}
	var detail SessionDetail
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return nil, err
	}
	return &detail, nil
}

func (b *httpBackend) Watch(
	ctx context.Context, id string,
) (<-chan Event, error) {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		b.baseURL+"/api/v1/sessions/"+url.PathEscape(id)+"/watch",
		nil,
	)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	b.addAuth(req)
	// Use a separate no-timeout client so long-lived streams do not
	// hit the 30s default on b.client.
	streamingClient := &http.Client{Timeout: 0}
	resp, err := streamingClient.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, fmt.Errorf("watch: session not found: %s", id)
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("watch: HTTP %d", resp.StatusCode)
	}

	out := make(chan Event)
	go func() {
		defer close(out)
		defer resp.Body.Close()
		// A dropped live-watch stream is signalled to the consumer by
		// closing out; there is no error channel, so a read error here
		// is not actionable.
		_ = parseSSE(resp.Body, func(ev Event) bool {
			select {
			case out <- ev:
				return true
			case <-ctx.Done():
				return false
			}
		})
	}()
	return out, nil
}

// Stats is not yet implemented over HTTP; the daemon currently has
// no /stats endpoint. Subsequent tasks may add one.
func (b *httpBackend) Stats(
	_ context.Context, _ StatsFilter,
) (*SessionStats, error) {
	return nil, errors.New("stats over HTTP backend: not yet implemented")
}

// Search proxies GET /api/v1/search. Every SearchRequest field maps to
// one query parameter; the empty query is rejected here so both backends
// fail identically without a round trip.
func (b *httpBackend) Search(
	ctx context.Context, req SearchRequest,
) (*SessionSearchResult, error) {
	query := strings.TrimSpace(req.Query)
	if query == "" {
		return nil, ErrSearchQueryRequired
	}
	q := url.Values{}
	q.Set("q", query)
	if req.Project != "" {
		q.Set("project", req.Project)
	}
	// Sort and cursor are left off when unset so the route applies its own
	// documented defaults (relevance, offset 0) instead of the client
	// pinning them. The limit is always sent: it is clamped here so the
	// direct backend and the daemon agree on the page size.
	if req.Sort != "" {
		q.Set("sort", req.Sort)
	}
	q.Set("limit", strconv.Itoa(clampSearchLimit(req.Limit)))
	if req.Cursor > 0 {
		q.Set("cursor", strconv.Itoa(req.Cursor))
	}
	var out SessionSearchResult
	err := b.getJSON(ctx, "/api/v1/search?"+q.Encode(), &out)
	var status *httpStatusError
	if errors.As(err, &status) &&
		status.Status == http.StatusNotImplemented {
		// The route sends 501 when its store reports HasFTS()==false.
		return nil, ErrSearchUnavailable
	}
	if err != nil {
		return nil, err
	}
	if out.Results == nil {
		out.Results = []db.SearchResult{}
	}
	return &out, nil
}

// usageQuery renders the shared usage filter as query parameters. Every
// UsageRequest field maps to exactly one parameter; dropping one here
// compiles and returns data, just for the wrong range or project.
//
// The two flags are only sent when the caller set them: an omitted
// parameter takes the route default, which is what a nil pointer means,
// so an unset request is identical on both transports.
func usageQuery(req UsageRequest) url.Values {
	q := url.Values{}
	for k, v := range map[string]string{
		"from":            req.From,
		"to":              req.To,
		"timezone":        req.Timezone,
		"agent":           req.Agent,
		"project":         req.Project,
		"machine":         req.Machine,
		"git_branch":      req.GitBranch,
		"exclude_project": req.ExcludeProject,
		"exclude_agent":   req.ExcludeAgent,
		"exclude_model":   req.ExcludeModel,
		"model":           req.Model,
		"active_since":    req.ActiveSince,
	} {
		if v != "" {
			q.Set(k, v)
		}
	}
	if req.MinUserMessages > 0 {
		q.Set("min_user_messages", strconv.Itoa(req.MinUserMessages))
	}
	if req.IncludeOneShot != nil {
		q.Set("include_one_shot", strconv.FormatBool(*req.IncludeOneShot))
	}
	if req.IncludeAutomated != nil {
		q.Set("include_automated", strconv.FormatBool(*req.IncludeAutomated))
	}
	if req.NoCache {
		q.Set("no_cache", "true")
	}
	return q
}

// UsageSummary proxies GET /api/v1/usage/summary. The request is
// validated with the shared rules first so both transports reject the
// same input with the same message and without a round trip.
func (b *httpBackend) UsageSummary(
	ctx context.Context, req UsageRequest,
) (*UsageSummaryResult, error) {
	if _, err := UsageFilterFromRequest(req); err != nil {
		return nil, err
	}
	var out UsageSummaryResult
	if err := b.getJSON(
		ctx, "/api/v1/usage/summary?"+usageQuery(req).Encode(), &out,
	); err != nil {
		return nil, err
	}
	return &out, nil
}

// UsagePairwiseComparison proxies GET
// /api/v1/usage/pairwise-comparison. The whole usage filter travels with
// the four side parameters: the route intersects each side against it,
// so a dropped filter param silently compares the wrong populations.
func (b *httpBackend) UsagePairwiseComparison(
	ctx context.Context, req UsagePairwiseComparisonRequest,
) (*PairwiseComparisonResponse, error) {
	if _, err := UsageFilterFromRequest(req.UsageRequest); err != nil {
		return nil, err
	}
	if _, err := ParsePairwiseDimension(string(req.LeftDimension)); err != nil {
		return nil, err
	}
	if _, err := ParsePairwiseDimension(string(req.RightDimension)); err != nil {
		return nil, err
	}
	q := usageQuery(req.UsageRequest)
	q.Set("left_dimension", string(req.LeftDimension))
	q.Set("left_value", req.LeftValue)
	q.Set("right_dimension", string(req.RightDimension))
	q.Set("right_value", req.RightValue)
	var out PairwiseComparisonResponse
	if err := b.getJSON(
		ctx, "/api/v1/usage/pairwise-comparison?"+q.Encode(), &out,
	); err != nil {
		return nil, err
	}
	return &out, nil
}

func (b *httpBackend) SearchContent(
	ctx context.Context, req ContentSearchRequest,
) (*ContentSearchResult, error) {
	q := url.Values{}
	q.Set("pattern", req.Pattern)
	if req.Mode != "" {
		q.Set("mode", req.Mode)
	}
	if len(req.Sources) > 0 {
		q.Set("in", strings.Join(req.Sources, ","))
	}
	if req.ExcludeSystem {
		q.Set("exclude_system", "true")
	}
	if req.Reveal {
		q.Set("reveal", "true")
	}
	for k, v := range map[string]string{
		"project":         req.Project,
		"exclude_project": req.ExcludeProject,
		"machine":         req.Machine,
		"git_branch":      req.GitBranch,
		"agent":           req.Agent,
		"date":            req.Date,
		"date_from":       req.DateFrom,
		"date_to":         req.DateTo,
		"active_since":    req.ActiveSince,
	} {
		if v != "" {
			q.Set(k, v)
		}
	}
	if req.IncludeChildren {
		q.Set("include_children", "true")
	}
	if req.IncludeAutomated {
		q.Set("include_automated", "true")
	}
	if req.IncludeOneShot {
		q.Set("include_one_shot", "true")
	}
	if req.Limit > 0 {
		q.Set("limit", strconv.Itoa(req.Limit))
	}
	if req.Cursor > 0 {
		q.Set("cursor", strconv.Itoa(req.Cursor))
	}
	var out ContentSearchResult
	if err := b.getJSON(ctx, "/api/v1/search/content?"+q.Encode(), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (b *httpBackend) ListSecrets(
	ctx context.Context, f SecretListFilter,
) (*SecretFindingList, error) {
	q := url.Values{}
	for k, v := range map[string]string{
		"project": f.Project, "agent": f.Agent,
		"date_from": f.DateFrom, "date_to": f.DateTo,
		"rule": f.Rule, "confidence": f.Confidence,
	} {
		if v != "" {
			q.Set(k, v)
		}
	}
	if f.Reveal {
		q.Set("reveal", "true")
	}
	if f.Limit > 0 {
		q.Set("limit", strconv.Itoa(f.Limit))
	}
	if f.Cursor > 0 {
		q.Set("cursor", strconv.Itoa(f.Cursor))
	}
	var out SecretFindingList
	if err := b.getJSON(ctx, "/api/v1/secrets?"+q.Encode(), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (b *httpBackend) ScanSecrets(
	ctx context.Context, in SecretScanInput,
	progress func(SecretScanProgress),
) (*SecretScanSummary, error) {
	if b.readOnly {
		return nil, fmt.Errorf("scan: daemon at %s is read-only: %w",
			b.baseURL, db.ErrReadOnly)
	}
	q := url.Values{}
	if in.Backfill {
		q.Set("backfill", "true")
	}
	for k, v := range map[string]string{
		"project": in.Project, "agent": in.Agent,
		"date_from": in.DateFrom, "date_to": in.DateTo,
	} {
		if v != "" {
			q.Set(k, v)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		b.baseURL+"/api/v1/secrets/scan?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Origin", b.baseURL)
	b.addAuth(req)
	resp, err := (&http.Client{Timeout: 0}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotImplemented {
		return nil, fmt.Errorf("scan: daemon at %s: %w", b.baseURL, db.ErrReadOnly)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("scan: HTTP %d", resp.StatusCode)
	}
	return parseScanStream(resp.Body, progress)
}

// parseScanStream decodes the scan SSE stream: progress ticks invoke the
// callback, the summary event is the result, and an error event becomes an
// error. A stream that ends without a summary event (broken connection,
// canceled context, daemon crash) is reported as an error rather than a
// zero-value success.
func parseScanStream(
	r io.Reader, progress func(SecretScanProgress),
) (*SecretScanSummary, error) {
	var summary SecretScanSummary
	var scanErr, decodeErr error
	var gotSummary bool
	readErr := parseSSE(r, func(ev Event) bool {
		switch ev.Event {
		case "progress":
			var p SecretScanProgress
			if json.Unmarshal([]byte(ev.Data), &p) == nil && progress != nil {
				progress(p)
			}
		case "summary":
			if err := json.Unmarshal([]byte(ev.Data), &summary); err != nil {
				decodeErr = fmt.Errorf("scan: decoding summary: %w", err)
			} else {
				gotSummary = true
			}
		case "error":
			scanErr = fmt.Errorf("scan: %s", ev.Data)
		}
		return true
	})
	switch {
	case scanErr != nil:
		// The server explicitly reported failure; prefer that over a
		// trailing read error from the dropped connection.
		return nil, scanErr
	case gotSummary:
		// A complete summary arrived; any post-summary read noise is
		// irrelevant to the scan result.
		return &summary, nil
	case readErr != nil:
		return nil, fmt.Errorf("scan: reading stream: %w", readErr)
	case decodeErr != nil:
		return nil, decodeErr
	default:
		return nil, errors.New("scan: stream ended before summary")
	}
}

// parseSSE reads a Server-Sent Events stream and invokes emit for
// each complete event. emit returns false to stop parsing (e.g. on
// context cancel). It returns any read error from the underlying
// stream, so callers can tell a truncated/broken stream apart from a
// clean end; a voluntary stop (emit returning false) returns nil.
func parseSSE(r io.Reader, emit func(Event) bool) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var event, data string
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if event != "" {
				if !emit(Event{Event: event, Data: data}) {
					return nil
				}
			}
			event, data = "", ""
			continue
		}
		if strings.HasPrefix(line, "event: ") {
			event = line[len("event: "):]
		} else if strings.HasPrefix(line, "data: ") {
			data = line[len("data: "):]
		}
	}
	return scanner.Err()
}

// addAuth attaches the bearer token to req when the backend was
// constructed with one. Safe to call on a request without a token
// configured (no-op).
func (b *httpBackend) addAuth(req *http.Request) {
	if b.token != "" {
		req.Header.Set("Authorization", "Bearer "+b.token)
	}
}

func (b *httpBackend) getJSON(
	ctx context.Context, path string, out any,
) error {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, b.baseURL+path, nil,
	)
	if err != nil {
		return err
	}
	b.addAuth(req)
	resp, err := b.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return errHTTPNotFound
	}
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return &httpStatusError{
			Path: path, Status: resp.StatusCode, Body: string(msg),
		}
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
