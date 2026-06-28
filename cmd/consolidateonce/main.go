// Command consolidateonce runs a deterministic, isolated consolidation trial.
// It is a verification harness, not an application command: it uses the real
// consolidate worker and the real dotfiles assist_consolidate.py script, while
// replacing remote LLM/embedding dependencies with local fakes.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.kenn.io/agentsview/internal/consolidate"
)

const (
	agentsviewRoot      = "/Users/zhenninglang/Projects/agentsview-lr-memory-consolidate-llm-gate"
	defaultDotfilesRoot = "/Users/zhenninglang/.dotfiles-lr-consolidate"
	realDotfilesRoot    = "/Users/zhenninglang/.dotfiles"
	rawDirRel           = "memory/.staging/raw_memories"
)

type evidence struct {
	Mode             string                       `json:"mode"`
	AgentsviewRoot   string                       `json:"agentsview_root"`
	DotfilesCodeRoot string                       `json:"dotfiles_code_root"`
	DataRoot         string                       `json:"data_root"`
	RunCWD           string                       `json:"run_cwd"`
	RawDir           string                       `json:"raw_dir"`
	StagingPath      string                       `json:"staging_path"`
	DBPath           string                       `json:"db_path"`
	AuditPath        string                       `json:"audit_path"`
	SafePaths        bool                         `json:"safe_paths"`
	RecallMode       string                       `json:"recall_mode"`
	RecallFailures   []string                     `json:"recall_failures,omitempty"`
	PromptChecks     map[string]bool              `json:"prompt_checks"`
	RunRecord        consolidate.RunRecord        `json:"run_record"`
	Candidates       map[string]candidateEvidence `json:"candidates"`
	Assertions       map[string]bool              `json:"assertions"`
	StdoutContains   map[string]bool              `json:"stdout_contains"`
	StderrContains   map[string]bool              `json:"stderr_contains"`
	Errors           []string                     `json:"errors,omitempty"`
}

type candidateEvidence struct {
	Action           string   `json:"action"`
	NoteID           string   `json:"note_id,omitempty"`
	Result           string   `json:"result,omitempty"`
	CreatedNotes     []string `json:"created_notes,omitempty"`
	TargetExists     bool     `json:"target_exists,omitempty"`
	TargetArchived   bool     `json:"target_archived,omitempty"`
	SupersededBy     string   `json:"superseded_by,omitempty"`
	ActiveNoteTextOK bool     `json:"active_note_text_ok,omitempty"`
}

type scriptedLLM struct {
	decisions map[string]consolidate.Decision
	prompt    string
}

func (l *scriptedLLM) ChatJSON(_ context.Context, _, user string) (string, error) {
	l.prompt = user
	data, err := json.Marshal(l.decisions)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

type scriptedRecaller struct {
	mode string
}

func (r scriptedRecaller) Recall(_ context.Context, c consolidate.Candidate, _ int) ([]consolidate.ExistingNote, error) {
	if r.mode == "fail" {
		return nil, errors.New("scripted recall unavailable")
	}
	switch c.ID {
	case "dup-background-jobs":
		return []consolidate.ExistingNote{{NoteID: "duplicate-target.md", Title: "BACKGROUND_JOBS_ENABLED=false disables background jobs", Status: "active", Excerpt: "Existing memory already records BACKGROUND_JOBS_ENABLED=false."}}, nil
	case "update-replacement":
		return []consolidate.ExistingNote{{NoteID: "update-target.md", Title: "Old update target", Status: "active", Excerpt: "Old guidance says memory consolidation does not need recall context."}}, nil
	case "delete-obsolete":
		return []consolidate.ExistingNote{{NoteID: "delete-target.md", Title: "Obsolete delete target", Status: "active", Excerpt: "Obsolete memory that should be invalidated."}}, nil
	default:
		return []consolidate.ExistingNote{}, nil
	}
}

type noOpCommitter struct{}

func (noOpCommitter) Commit(context.Context, string) error { return nil }

type noOpResyncer struct{}

func (noOpResyncer) Resync(context.Context) error { return nil }

func main() {
	var mode string
	var dotfilesRoot string
	var jsonOut string
	flag.StringVar(&mode, "mode", "normal", "trial mode: normal or failopen")
	flag.StringVar(&dotfilesRoot, "dotfiles-root", defaultDotfilesRoot, "dotfiles code worktree")
	flag.StringVar(&jsonOut, "json-out", "", "optional path to write JSON evidence")
	flag.Parse()

	ev, err := run(context.Background(), mode, dotfilesRoot)
	if err != nil {
		ev.Errors = append(ev.Errors, err.Error())
	}
	data, marshalErr := json.MarshalIndent(ev, "", "  ")
	if marshalErr != nil {
		fmt.Fprintf(os.Stderr, "marshal evidence: %v\n", marshalErr)
		os.Exit(1)
	}
	if jsonOut != "" {
		if err := os.MkdirAll(filepath.Dir(jsonOut), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "mkdir json-out: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(jsonOut, append(data, '\n'), 0o600); err != nil {
			fmt.Fprintf(os.Stderr, "write json-out: %v\n", err)
			os.Exit(1)
		}
	}
	fmt.Println(string(data))
	passed := err == nil && allAssertionsPass(ev.Assertions)
	if passed {
		_ = os.RemoveAll(ev.DataRoot)
		_ = os.RemoveAll(ev.RunCWD)
	}
	if !passed {
		os.Exit(1)
	}
}

func run(ctx context.Context, mode, dotfilesRoot string) (evidence, error) {
	if mode != "normal" && mode != "failopen" {
		return evidence{Mode: mode}, fmt.Errorf("unsupported mode %q", mode)
	}
	dotfilesRoot, err := filepath.Abs(dotfilesRoot)
	if err != nil {
		return evidence{Mode: mode}, err
	}
	dataRoot, err := os.MkdirTemp("", "agentsview-consolidate-e2e-")
	if err != nil {
		return evidence{Mode: mode}, err
	}
	runCWD, err := os.MkdirTemp("", "agentsview-consolidate-cwd-")
	if err != nil {
		return evidence{Mode: mode, DataRoot: dataRoot}, err
	}
	if err := setupRoot(dataRoot, dotfilesRoot); err != nil {
		return evidence{Mode: mode, DataRoot: dataRoot, DotfilesCodeRoot: dotfilesRoot}, err
	}
	if err := seedExistingNotes(dataRoot); err != nil {
		return evidence{Mode: mode, DataRoot: dataRoot, DotfilesCodeRoot: dotfilesRoot}, err
	}
	if mode == "normal" {
		err = seedNormalCandidates(filepath.Join(dataRoot, rawDirRel))
	} else {
		err = seedFailOpenCandidates(filepath.Join(dataRoot, rawDirRel))
	}
	if err != nil {
		return evidence{Mode: mode, DataRoot: dataRoot, DotfilesCodeRoot: dotfilesRoot}, err
	}

	llm := &scriptedLLM{decisions: decisionsFor(mode)}
	recallMode := "scripted"
	recaller := scriptedRecaller{mode: "normal"}
	if mode == "failopen" {
		recallMode = "scripted-fail"
		recaller.mode = "fail"
	}

	oldCWD, _ := os.Getwd()
	_ = os.Chdir(runCWD)
	defer func() { _ = os.Chdir(oldCWD) }()

	worker := consolidate.NewWorker(
		filepath.Join(dataRoot, "memory", ".staging"),
		filepath.Join(dataRoot, rawDirRel),
		dataRoot,
		llm,
		consolidate.PythonScriptRunner{},
		noOpCommitter{},
		noOpResyncer{},
		consolidate.NewAuditLog(filepath.Join(dataRoot, "memory", consolidate.AuditPath(""))),
		recaller,
	)

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	rec, runErr := worker.RunOnce(ctx)

	sanitized := sanitizeRunRecord(rec)
	ev := evidence{
		Mode:             mode,
		AgentsviewRoot:   agentsviewRoot,
		DotfilesCodeRoot: dotfilesRoot,
		DataRoot:         dataRoot,
		RunCWD:           runCWD,
		RawDir:           filepath.Join(dataRoot, rawDirRel),
		StagingPath:      filepath.Join(dataRoot, "memory", ".staging"),
		DBPath:           filepath.Join(dataRoot, "agentsview-e2e.sqlite"),
		AuditPath:        filepath.Join(dataRoot, "memory", consolidate.AuditPath("")),
		SafePaths:        pathsAreSafe(dataRoot, dotfilesRoot),
		RecallMode:       recallMode,
		PromptChecks:     promptChecks(mode, llm.prompt),
		RunRecord:        sanitized,
		Candidates:       collectCandidateEvidence(dataRoot, rec),
		Assertions:       map[string]bool{},
		StdoutContains:   containsMap(joinResults(rec.Decisions), []string{"missing per-candidate decision", "write background-jobs", "write dev-complete-required", "update update-replacement", "soft_invalidate delete-obsolete"}),
		StderrContains:   containsMap(strings.Join(rec.ScriptErrors, "\n"), []string{"missing per-candidate decision", "redact"}),
	}
	if mode == "failopen" && rec.Note != "" {
		ev.RecallFailures = []string{rec.Note}
	}
	ev.Assertions = assertionsFor(mode, ev)
	return ev, runErr
}

func sanitizeRunRecord(rec consolidate.RunRecord) consolidate.RunRecord {
	if len(rec.ScriptErrors) == 0 {
		return rec
	}
	sanitized := make([]string, 0, len(rec.ScriptErrors))
	for _, line := range rec.ScriptErrors {
		switch {
		case strings.Contains(strings.ToLower(line), "redact"):
			sanitized = append(sanitized, "assist_consolidate failed: redact gate rejected candidate")
		case strings.Contains(line, "missing per-candidate decision"):
			sanitized = append(sanitized, "assist_consolidate failed: missing per-candidate decision")
		default:
			sanitized = append(sanitized, "assist_consolidate failed: sanitized error")
		}
	}
	rec.ScriptErrors = sanitized
	return rec
}

func setupRoot(root, dotfilesRoot string) error {
	if !pathsAreSafe(root, dotfilesRoot) {
		return fmt.Errorf("unsafe data root %s", root)
	}
	for _, dir := range []string{filepath.Join(root, "memory", "user"), filepath.Join(root, rawDirRel)} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	for _, name := range []string{"coding-skills", "scripts"} {
		if err := os.Symlink(filepath.Join(dotfilesRoot, name), filepath.Join(root, name)); err != nil {
			return err
		}
	}
	return nil
}

func pathsAreSafe(dataRoot, dotfilesRoot string) bool {
	dataRoot = clean(dataRoot)
	for _, forbidden := range []string{dotfilesRoot, realDotfilesRoot, agentsviewRoot} {
		if forbidden == "" {
			continue
		}
		forbidden = clean(forbidden)
		if dataRoot == forbidden || strings.HasPrefix(dataRoot, forbidden+string(os.PathSeparator)) {
			return false
		}
	}
	return strings.HasPrefix(dataRoot, clean(os.TempDir())+string(os.PathSeparator))
}

func clean(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return real
	}
	return filepath.Clean(abs)
}

func seedExistingNotes(root string) error {
	userDir := filepath.Join(root, "memory", "user")
	notes := map[string]string{
		"duplicate-target.md": "BACKGROUND_JOBS_ENABLED=false disables background jobs\n\nEvidence: Existing memory already records BACKGROUND_JOBS_ENABLED=false.\n\nImplication: Do not add another active duplicate.\n",
		"update-target.md":    "Old update target\n\nEvidence: Old guidance says memory consolidation does not need recall context.\n\nImplication: This should be superseded by corrected recall guidance.\n",
		"delete-target.md":    "Obsolete delete target\n\nEvidence: This memory is now contradicted by newer evidence.\n\nImplication: This should be soft-invalidated rather than removed.\n",
	}
	for name, body := range notes {
		text := "---\n" +
			"title: " + strings.TrimSuffix(strings.ReplaceAll(name, ".md", ""), "\n") + "\n" +
			"date: 2026-06-28\n" +
			"problem_type: knowledge\n" +
			"type: semantic\n" +
			"status: active\n" +
			"valid_from: 2026-06-28\n" +
			"valid_to:\n" +
			"superseded_by:\n" +
			"keywords: [memory, consolidate]\n" +
			"origin_session: e2e-existing\n" +
			"verify:\n" +
			"---\n\n" + body
		if err := os.WriteFile(filepath.Join(userDir, name), []byte(text), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func seedNormalCandidates(rawDir string) error {
	candidates := []map[string]any{
		candidate("dup-background-jobs", "fact", "BACKGROUND_JOBS_ENABLED=false disables background jobs", "Trial candidate repeats the existing BACKGROUND_JOBS_ENABLED=false memory.", "Future agents should not add another active duplicate for BACKGROUND_JOBS_ENABLED=false."),
		candidate("update-replacement", "correction", "Memory consolidation uses recall context for update/delete", "New evidence shows the LLM receives similar_memories before choosing UPDATE.", "Replace stale guidance that claimed consolidation lacks recall context."),
		candidate("delete-obsolete", "fact", "Obsolete memory should be invalidated", "Newer evidence contradicts the delete target.", "Soft-invalidate the old target instead of deleting the file."),
		candidate("background-jobs", "fact", "BACKGROUND_JOBS_ENABLED=false disables background jobs", "Trial memory candidate recorded BACKGROUND_JOBS_ENABLED=false as an operator-relevant fact.", "When background processing appears inactive, check BACKGROUND_JOBS_ENABLED before debugging workers."),
		candidate("dev-complete-required", "correction", "Code changes must go through /dev-complete", "Trial memory candidate recorded the user rule: 代码改动必须走 /dev-complete.", "For future code changes, choose /dev-complete unless the task is a pure bug fix or otherwise scoped out."),
		candidate("secret-like", "fact", "Secret-like candidate must be rejected", "The production token is "+secretToken(), "The deterministic safety gate must reject the candidate before writing memory."),
	}
	return writeCandidates(rawDir, candidates)
}

func seedFailOpenCandidates(rawDir string) error {
	return writeCandidates(rawDir, []map[string]any{
		candidate("degraded-knowledge", "fact", "Degraded recall still allows safe consolidation", "The recall dependency is unavailable in this route.", "A complete useful candidate can still be semantically added with empty similar_memories."),
	})
}

func candidate(id, category, summary, evidence, implication string) map[string]any {
	return map[string]any{
		"id":              id,
		"summary":         summary,
		"evidence":        evidence,
		"implication":     implication,
		"category":        category,
		"problem_type":    category,
		"origin_session":  "phase-04-e2e",
		"source_platform": "agentsview-e2e",
		"source_roles":    []string{"user", "assistant"},
		"created_at":      "2026-06-28T00:00:00Z",
		"why":             "",
		"occurrences":     1,
	}
}

func writeCandidates(rawDir string, candidates []map[string]any) error {
	if err := os.MkdirAll(rawDir, 0o700); err != nil {
		return err
	}
	for _, c := range candidates {
		data, err := json.MarshalIndent(c, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(rawDir, c["id"].(string)+".json"), append(data, '\n'), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func secretToken() string {
	parts := []string{"The production", " token is ", "placeholder-value-long-enough"}
	return strings.Join(parts, "")
}

func decisionsFor(mode string) map[string]consolidate.Decision {
	if mode == "failopen" {
		return map[string]consolidate.Decision{"degraded-knowledge": {Action: consolidate.ActionADD, Reason: "complete useful knowledge despite empty context"}}
	}
	return map[string]consolidate.Decision{
		"dup-background-jobs":   {Action: consolidate.ActionSKIP, Reason: "duplicate of duplicate-target.md"},
		"update-replacement":    {Action: consolidate.ActionUPDATE, NoteID: "update-target.md", Reason: "supersedes stale target"},
		"delete-obsolete":       {Action: consolidate.ActionDELETE, NoteID: "delete-target.md", Reason: "obsolete target"},
		"background-jobs":       {Action: consolidate.ActionADD, Reason: "new trial knowledge"},
		"dev-complete-required": {Action: consolidate.ActionADD, Reason: "new reusable user rule"},
		"secret-like":           {Action: consolidate.ActionADD, Reason: "semantic add, deterministic safety should reject"},
	}
}

func promptChecks(mode, prompt string) map[string]bool {
	checks := map[string]bool{"has_similar_memories_field": strings.Contains(prompt, "similar_memories")}
	if mode == "normal" {
		checks["duplicate_has_context"] = strings.Contains(prompt, "dup-background-jobs") && strings.Contains(prompt, "duplicate-target.md")
		checks["update_has_context"] = strings.Contains(prompt, "update-replacement") && strings.Contains(prompt, "update-target.md")
		checks["delete_has_context"] = strings.Contains(prompt, "delete-obsolete") && strings.Contains(prompt, "delete-target.md")
	} else {
		checks["degraded_empty_context"] = strings.Contains(prompt, "degraded-knowledge") && strings.Contains(prompt, "\"similar_memories\":[]")
	}
	return checks
}

func collectCandidateEvidence(root string, rec consolidate.RunRecord) map[string]candidateEvidence {
	out := make(map[string]candidateEvidence, len(rec.Decisions))
	for _, d := range rec.Decisions {
		ev := candidateEvidence{Action: d.Action, NoteID: d.NoteID, Result: d.Result}
		switch d.CandidateID {
		case "background-jobs":
			ev.CreatedNotes, ev.ActiveNoteTextOK = noteFromResultContains(root, d.Result, "BACKGROUND_JOBS_ENABLED=false")
		case "dev-complete-required":
			ev.CreatedNotes, ev.ActiveNoteTextOK = noteFromResultContains(root, d.Result, "代码改动必须走 /dev-complete")
		case "update-replacement":
			ev.CreatedNotes, ev.ActiveNoteTextOK = noteFromResultContains(root, d.Result, "New evidence shows the LLM receives similar_memories")
			ev.TargetExists, ev.TargetArchived, ev.SupersededBy = targetState(root, "update-target.md")
		case "delete-obsolete":
			ev.TargetExists, ev.TargetArchived, ev.SupersededBy = targetState(root, "delete-target.md")
		case "degraded-knowledge":
			ev.CreatedNotes, ev.ActiveNoteTextOK = noteFromResultContains(root, d.Result, "Degraded recall still allows safe consolidation")
		}
		out[d.CandidateID] = ev
	}
	return out
}

func noteFromResultContains(root, result, needle string) ([]string, bool) {
	rel := writtenNoteRelPath(result)
	if rel == "" {
		return nil, false
	}
	path := filepath.Join(root, filepath.FromSlash(rel))
	text := read(path)
	if strings.Contains(text, needle) && strings.Contains(text, "status: active") {
		return []string{filepath.Base(path)}, true
	}
	return []string{filepath.Base(path)}, false
}

func writtenNoteRelPath(result string) string {
	fields := strings.Fields(result)
	if len(fields) >= 3 && fields[0] == "write" {
		return fields[2]
	}
	if len(fields) >= 5 && fields[0] == "update" && fields[3] == "->" {
		return fields[4]
	}
	return ""
}

func targetState(root, name string) (exists, archived bool, supersededBy string) {
	path := filepath.Join(root, "memory", "user", name)
	textBytes, err := os.ReadFile(path)
	if err != nil {
		return false, false, ""
	}
	text := string(textBytes)
	return true, strings.Contains(text, "status: archived"), frontmatterValue(text, "superseded_by")
}

func frontmatterValue(text, key string) string {
	for _, line := range strings.Split(text, "\n") {
		left, right, ok := strings.Cut(line, ":")
		if ok && strings.TrimSpace(left) == key {
			return strings.TrimSpace(right)
		}
	}
	return ""
}

func userNotePaths(root string) []string {
	entries, _ := os.ReadDir(filepath.Join(root, "memory", "user"))
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" || entry.Name() == "INDEX.md" {
			continue
		}
		paths = append(paths, filepath.Join(root, "memory", "user", entry.Name()))
	}
	sort.Strings(paths)
	return paths
}

func assertionsFor(mode string, ev evidence) map[string]bool {
	assertions := map[string]bool{
		"safe_paths":                          ev.SafePaths,
		"run_cwd_outside_dotfiles_and_data":   !inside(ev.RunCWD, ev.DotfilesCodeRoot) && !inside(ev.RunCWD, ev.DataRoot),
		"no_missing_per_candidate_decision":   !ev.StdoutContains["missing per-candidate decision"] && !ev.StderrContains["missing per-candidate decision"] && !strings.Contains(ev.RunRecord.Error, "missing per-candidate decision"),
		"worker_returned_without_fatal_error": ev.RunRecord.Error == "",
	}
	if mode == "normal" {
		assertions["promoted_count_gt_zero"] = len(candidateCreatedNotes(ev)) > 0
		assertions["duplicate_skip"] = ev.Candidates["dup-background-jobs"].Action == "SKIP" && !strings.Contains(joinCandidateCreatedNoteNames(ev), "dup-background-jobs")
		upd := ev.Candidates["update-replacement"]
		assertions["update_soft_archives_target"] = upd.Action == "UPDATE" && upd.NoteID == "update-target.md" && upd.TargetExists && upd.TargetArchived && upd.SupersededBy != "" && upd.ActiveNoteTextOK
		del := ev.Candidates["delete-obsolete"]
		assertions["delete_soft_invalidates_target"] = (del.Action == "DELETE" || del.Action == "INVALIDATE") && del.NoteID == "delete-target.md" && del.TargetExists && del.TargetArchived && del.SupersededBy != ""
		assertions["background_jobs_add"] = ev.Candidates["background-jobs"].Action == "ADD" && ev.Candidates["background-jobs"].ActiveNoteTextOK
		assertions["dev_complete_add"] = ev.Candidates["dev-complete-required"].Action == "ADD" && ev.Candidates["dev-complete-required"].ActiveNoteTextOK
		assertions["secret_rejected_no_note"] = ev.Candidates["secret-like"].Action == "ADD" && ev.StderrContains["redact"] && !memoryContains(ev.DataRoot, secretToken())
		assertions["prompt_has_update_delete_context"] = ev.PromptChecks["duplicate_has_context"] && ev.PromptChecks["update_has_context"] && ev.PromptChecks["delete_has_context"]
	} else {
		assertions["recall_failure_recorded"] = strings.Contains(ev.RunRecord.Note, "memory recall failed")
		assertions["degraded_add_written"] = ev.Candidates["degraded-knowledge"].Action == "ADD" && ev.Candidates["degraded-knowledge"].ActiveNoteTextOK
		assertions["prompt_empty_context"] = ev.PromptChecks["degraded_empty_context"]
	}
	return assertions
}

func inside(path, root string) bool {
	path, root = clean(path), clean(root)
	return path == root || strings.HasPrefix(path, root+string(os.PathSeparator))
}

func candidateCreatedNotes(ev evidence) []string {
	var names []string
	for _, cand := range ev.Candidates {
		names = append(names, cand.CreatedNotes...)
	}
	return names
}

func joinCandidateCreatedNoteNames(ev evidence) string {
	return strings.Join(candidateCreatedNotes(ev), "\n")
}

func memoryContains(root, needle string) bool {
	for _, path := range userNotePaths(root) {
		if strings.Contains(read(path), needle) {
			return true
		}
	}
	return false
}

func read(path string) string {
	data, _ := os.ReadFile(path)
	return string(data)
}

func joinResults(records []consolidate.DecisionRecord) string {
	var lines []string
	for _, rec := range records {
		lines = append(lines, rec.Result)
	}
	return strings.Join(lines, "\n")
}

func containsMap(haystack string, needles []string) map[string]bool {
	out := make(map[string]bool, len(needles))
	for _, needle := range needles {
		out[needle] = strings.Contains(haystack, needle)
	}
	return out
}

func allAssertionsPass(assertions map[string]bool) bool {
	for _, ok := range assertions {
		if !ok {
			return false
		}
	}
	return true
}
