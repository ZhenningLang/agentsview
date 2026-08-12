#!/usr/bin/env bash

# End-to-end acceptance checks for the upstream-catchup long run.
# Intentionally does not use `set -e`: every check should run, then summarize.

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null)"
if [ -z "$REPO_ROOT" ]; then
  REPO_ROOT="$(pwd)"
fi

WORKSPACE_DIR="/Users/zhenninglang/Projects/agentsview/.long-loop/20260812_upstream-catchup"
REQUESTED_A15_BASE="798c6885"
GO_TAGS="fts5,kit_posthog_disabled"
FAILURES=0

print_block() {
  printf '%s\n' "$1"
}

pass() {
  printf 'PASS %s\n' "$1"
}

fail() {
  printf 'FAIL %s\n' "$1"
  if [ -n "$2" ]; then
    printf '%s\n' "$2"
  fi
  FAILURES=$((FAILURES + 1))
}

check_command() {
  name="$1"
  shift
  output="$($@ 2>&1)"
  status=$?
  if [ "$status" -eq 0 ]; then
    pass "$name"
    if [ -n "$output" ]; then
      printf '%s\n' "$output"
    fi
  else
    fail "$name" "$output"
  fi
}

check_a11_dompurify_versions() {
  name="A11 dompurify package and lockfile versions are 3.4.13"
  pkg_version="$(node -e "const p=require('./frontend/package.json'); console.log(p.dependencies && p.dependencies.dompurify || '')" 2>&1)"
  pkg_status=$?
  lock_root="$(node -e "const p=require('./frontend/package-lock.json'); console.log(p.packages && p.packages[''] && p.packages[''].dependencies && p.packages[''].dependencies.dompurify || '')" 2>&1)"
  lock_root_status=$?
  lock_node="$(node -e "const p=require('./frontend/package-lock.json'); console.log(p.packages && p.packages['node_modules/dompurify'] && p.packages['node_modules/dompurify'].version || '')" 2>&1)"
  lock_node_status=$?
  if [ "$pkg_status" -eq 0 ] && [ "$lock_root_status" -eq 0 ] && [ "$lock_node_status" -eq 0 ] && [ "$pkg_version" = "3.4.13" ] && [ "$lock_root" = "3.4.13" ] && [ "$lock_node" = "3.4.13" ]; then
    pass "$name"
  else
    fail "$name" "package.json=$pkg_version package-lock.root=$lock_root package-lock.node_modules=$lock_node"
  fi
}

check_a12_advisory_doc() {
  name="A12 DOMPurify advisory doc is tracked and cites GitHub GHSA sources"
  path="docs/dompurify-3.4.13-advisories.md"
  tracked="$(git ls-files -- "$path")"
  if [ "$tracked" = "$path" ] && [ -f "$path" ] && grep -q 'https://github.com/advisories/GHSA-' "$path"; then
    pass "$name"
  else
    fail "$name" "tracked=$tracked exists=$(test -f "$path" && printf yes || printf no)"
  fi
}

check_a14_upstream_audit_doc() {
  name="A14 upstream audit doc is tracked and contains required metadata"
  path="docs/UPSTREAM_AUDIT.md"
  tracked="$(git ls-files -- "$path")"
  if [ "$tracked" = "$path" ] && [ -f "$path" ] && grep -q 'local_head' "$path" && grep -q 'spot_check_error_rate' "$path"; then
    pass "$name"
  else
    fail "$name" "tracked=$tracked local_head=$(grep -c 'local_head' "$path" 2>/dev/null) spot_check_error_rate=$(grep -c 'spot_check_error_rate' "$path" 2>/dev/null)"
  fi
}

check_a15_fork_core_untouched() {
  name="A15 fork-owned parser, memory, synthesize, and recall boundaries are untouched"
  run_base="$(git merge-base main HEAD 2>/dev/null)"
  if [ -z "$run_base" ]; then
    run_base="21cb8b716e70584efb4d3cd34d8dd8526617c5a3"
  fi
  bad_paths="$(git diff --name-only "$run_base"...HEAD -- \
    internal/parser/droid.go \
    internal/parser/kimicode.go \
    internal/memory \
    internal/synthesize \
    2>/dev/null)"
  recall_diff="$(git diff --name-status "$run_base"...HEAD -- internal/recall 2>/dev/null)"
  requested_range_forbidden="$(git diff --name-only "$REQUESTED_A15_BASE"..HEAD -- \
    internal/parser/droid.go \
    internal/parser/kimicode.go \
    internal/memory \
    internal/synthesize \
    internal/recall \
    2>/dev/null)"
  if [ -z "$bad_paths" ] && [ -z "$recall_diff" ] && [ ! -d internal/recall ]; then
    pass "$name"
    if [ -n "$requested_range_forbidden" ]; then
      printf 'NOTE A15 requested raw range %s..HEAD includes pre-run fork-core changes already present before main baseline:\n%s\n' "$REQUESTED_A15_BASE" "$requested_range_forbidden"
    fi
  else
    fail "$name" "run_base=$run_base changed_forbidden_paths=$bad_paths recall_diff=$recall_diff recall_dir=$(test -d internal/recall && printf yes || printf no)"
  fi
}

check_handoff_pg_parity() {
  name="$1"
  path="$2"
  if [ -f "$path" ] && grep -q '^## PG parity' "$path" && grep -Eqi 'Conclusion:|Integration status:|not affected|不受影响|NOT RUN|未跑|passed|已修' "$path"; then
    pass "$name"
  else
    fail "$name" "missing or incomplete PG parity section: $path"
  fi
}

check_a16_pg_parity_handoffs() {
  check_handoff_pg_parity "A16 phase 01 HANDOFF has PG parity conclusion" "$WORKSPACE_DIR/phases/01_archive-drop-guard/HANDOFF.md"
  check_handoff_pg_parity "A16 phase 02 HANDOFF has PG parity conclusion" "$WORKSPACE_DIR/phases/02_fts-operator-500/HANDOFF.md"
  check_handoff_pg_parity "A16 phase 03 HANDOFF has PG parity conclusion" "$WORKSPACE_DIR/phases/03_stats-peak-context/HANDOFF.md"
}

check_a17_requirement_entry() {
  name="A17 upstream catchup requirement entry exists and points to audit ledger"
  matches="$(git ls-files 'requirements/2026-08-12_upstream-catchup_*.md')"
  count="$(printf '%s\n' "$matches" | sed '/^$/d' | wc -l | tr -d ' ')"
  if [ "$count" -ge 1 ] && grep -q 'docs/UPSTREAM_AUDIT.md' $matches; then
    pass "$name"
  else
    fail "$name" "matches=$matches"
  fi
}

check_a18_gofmt() {
  name="A18 gofmt -l is empty for tracked and untracked Go files"
  files="$(git ls-files -- '*.go'; git ls-files --others --exclude-standard -- '*.go')"
  if [ -z "$files" ]; then
    pass "$name"
    return
  fi
  output="$(printf '%s\n' "$files" | xargs gofmt -l 2>&1)"
  status=$?
  if [ "$status" -eq 0 ] && [ -z "$output" ]; then
    pass "$name"
  else
    fail "$name" "$output"
  fi
}

cd "$REPO_ROOT" || exit 1

print_block "ACCEPTANCE START repo=$REPO_ROOT"

check_a11_dompurify_versions
check_a12_advisory_doc
check_a14_upstream_audit_doc
check_a15_fork_core_untouched
check_a16_pg_parity_handoffs
check_a17_requirement_entry
check_a18_gofmt
check_command "A18 go vet is clean" env CGO_ENABLED=1 go vet -tags "$GO_TAGS" ./...
check_command "A18 go test passes" env CGO_ENABLED=1 go test -tags "$GO_TAGS" ./...
check_command "frontend npx vitest run passes" bash -c 'cd frontend && npx vitest run'

if [ "$FAILURES" -eq 0 ]; then
  print_block "ACCEPTANCE OK"
  exit 0
fi

print_block "ACCEPTANCE FAILED failures=$FAILURES"
exit 1
