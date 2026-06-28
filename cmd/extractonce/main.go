// Command extractonce runs a deterministic, isolated extraction trial.
// It is a lightweight harness for compiling and exercising the extract worker
// without opening the real agentsview database or calling a remote LLM.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"go.kenn.io/agentsview/internal/db"
	"go.kenn.io/agentsview/internal/extract"
)

type fakeStore struct{}

func (fakeStore) ListSessions(context.Context, db.SessionFilter) (db.SessionPage, error) {
	first := "User asked to remember that deterministic extraction harnesses use isolated temp roots."
	return db.SessionPage{Sessions: []db.Session{{ID: "extract-e2e-session", Agent: "kilo", Project: "agentsview", FirstMessage: &first}}}, nil
}

func (fakeStore) GetAllMessages(context.Context, string) ([]db.Message, error) {
	return []db.Message{
		{SessionID: "extract-e2e-session", Ordinal: 1, Role: "user", Content: "Remember that extractonce must not write real staging data."},
		{SessionID: "extract-e2e-session", Ordinal: 2, Role: "assistant", Content: "Use an isolated temp root and gitignored staging."},
	}, nil
}

type fakeLLM struct{}

func (fakeLLM) ChatJSON(context.Context, string, string) (string, error) {
	return `{"candidates":[{"category":"decision","summary":"extractonce uses isolated staging","why":"It verifies the harness without production DB or remote LLM dependencies.","evidence":"The harness creates a temporary root and fake session store.","implication":"Future e2e extraction checks should keep raw candidates under temp memory/.staging only."}]}`, nil
}

type evidence struct {
	AgentsviewRoot string            `json:"agentsview_root"`
	DataRoot       string            `json:"data_root"`
	RawDir         string            `json:"raw_dir"`
	RunRecord      extract.RunRecord `json:"run_record"`
	SafePaths      bool              `json:"safe_paths"`
}

func main() {
	var jsonOut string
	flag.StringVar(&jsonOut, "json-out", "", "optional path to write JSON evidence")
	flag.Parse()

	root, err := os.MkdirTemp("", "agentsview-extract-e2e-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mktemp: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("memory/.staging/\n"), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "write gitignore: %v\n", err)
		os.Exit(1)
	}
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "git init: %v\n%s", err, out)
		os.Exit(1)
	}
	worker := extract.NewWorker(fakeStore{}, fakeLLM{}, root, extract.NewAuditLog(filepath.Join(root, "memory", extract.AuditPath(""))))
	rec, err := worker.RunOnce(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "run extract worker: %v\n", err)
		os.Exit(1)
	}
	ev := evidence{
		AgentsviewRoot: "/Users/zhenninglang/Projects/agentsview-lr-memory-consolidate-llm-gate",
		DataRoot:       root,
		RawDir:         extract.RawDir(root),
		RunRecord:      rec,
		SafePaths:      strings.HasPrefix(clean(root), clean(os.TempDir())+string(os.PathSeparator)),
	}
	data, err := json.MarshalIndent(ev, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "marshal evidence: %v\n", err)
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
	passed := ev.SafePaths && rec.Written > 0 && rec.StagingFiles > 0 && rec.Error == ""
	if passed {
		_ = os.RemoveAll(root)
	}
	if !passed {
		os.Exit(1)
	}
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
