package parser

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/tidwall/gjson"
)

const maxSkillFrontmatterSize = 64 << 10

var shellSegmentRE = regexp.MustCompile(`\s*(?:&&|\|\||;|&|\|)\s*`)

func inferToolSkillName(toolName, inputJSON string) string {
	if name := inferCursorSkillName(toolName, inputJSON); name != "" {
		return name
	}
	return inferCodexSkillNameWithBase(toolName, inputJSON, "")
}

func inferCursorSkillName(toolName, inputJSON string) string {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "read", "readfile", "read_file":
		return inferSkillNameFromJSONPaths(inputJSON, "")
	default:
		return ""
	}
}

func inferCodexSkillNameWithBase(toolName, inputJSON, fallbackBaseDir string) string {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "exec_command", "shell_command", "shell", "bash":
	default:
		return ""
	}
	cmd := skillCommandFromInput(inputJSON)
	if !strings.Contains(cmd, "SKILL.md") {
		return ""
	}
	baseDir := skillBaseDirFromInput(inputJSON)
	if baseDir == "" {
		baseDir = fallbackBaseDir
	}
	for _, path := range skillPathsFromCommand(cmd) {
		if name := skillNameFromPath(path, baseDir); name != "" {
			return name
		}
	}
	return ""
}

func skillCommandFromInput(inputJSON string) string {
	trimmed := strings.TrimSpace(inputJSON)
	if trimmed == "" {
		return ""
	}
	if gjson.Valid(trimmed) {
		g := gjson.Parse(trimmed)
		for _, key := range []string{"cmd", "command", "script"} {
			if s := strings.TrimSpace(g.Get(key).Str); s != "" {
				return s
			}
		}
	}
	return trimmed
}

func skillBaseDirFromInput(inputJSON string) string {
	trimmed := strings.TrimSpace(inputJSON)
	if trimmed == "" || !gjson.Valid(trimmed) {
		return ""
	}
	g := gjson.Parse(trimmed)
	for _, key := range []string{"workdir", "cwd", "working_directory"} {
		if s := strings.TrimSpace(g.Get(key).Str); s != "" {
			return s
		}
	}
	return ""
}

func inferSkillNameFromJSONPaths(inputJSON, fallbackBaseDir string) string {
	trimmed := strings.TrimSpace(inputJSON)
	if trimmed == "" {
		return ""
	}
	baseDir := skillBaseDirFromInput(trimmed)
	if baseDir == "" {
		baseDir = fallbackBaseDir
	}
	if !gjson.Valid(trimmed) {
		return skillNameFromPath(trimmed, baseDir)
	}
	var v any
	if err := json.Unmarshal([]byte(trimmed), &v); err != nil {
		return ""
	}
	return inferSkillNameFromValue(v, baseDir)
}

func inferSkillNameFromValue(v any, baseDir string) string {
	switch x := v.(type) {
	case string:
		return skillNameFromPath(x, baseDir)
	case []any:
		for _, item := range x {
			if name := inferSkillNameFromValue(item, baseDir); name != "" {
				return name
			}
		}
	case map[string]any:
		for _, item := range x {
			if name := inferSkillNameFromValue(item, baseDir); name != "" {
				return name
			}
		}
	}
	return ""
}

func skillPathsFromCommand(cmd string) []string {
	var out []string
	for _, seg := range shellSegmentRE.Split(cmd, -1) {
		out = append(out, skillPathsFromSegment(seg)...)
	}
	return out
}

func skillPathsFromSegment(seg string) []string {
	tokens := tokenizeCommand(seg)
	if len(tokens) == 0 {
		return nil
	}
	verb := commandVerb(tokens[0])
	args := tokens[1:]
	switch verb {
	case "cat", "head", "tail", "less", "more":
		return skillFilePaths(args)
	case "sed":
		if sedWritesInPlace(args) {
			return nil
		}
		return skillFilePaths(args)
	case "grep", "rg":
		return skillPathsFromSearchArgs(args)
	default:
		return nil
	}
}

func tokenizeCommand(seg string) []string {
	var tokens []string
	var cur strings.Builder
	var quote rune
	inToken := false
	skipNext := false
	flush := func() {
		if !inToken {
			return
		}
		if skipNext {
			skipNext = false
		} else {
			tokens = append(tokens, cur.String())
		}
		cur.Reset()
		inToken = false
	}
	for _, r := range seg {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			inToken = true
		case r == ' ' || r == '\t' || r == '\n' || r == '\r':
			flush()
		case r == '>':
			flush()
			skipNext = true
		default:
			cur.WriteRune(r)
			inToken = true
		}
	}
	flush()
	return tokens
}

func commandVerb(token string) string {
	token = strings.ToLower(token)
	if i := strings.LastIndexAny(token, `/\`); i >= 0 {
		token = token[i+1:]
	}
	return token
}

func skillFilePaths(args []string) []string {
	var paths []string
	for _, arg := range args {
		if isSkillMarkdownPath(arg) && !skillPathIsGlob(arg) {
			paths = append(paths, arg)
		}
	}
	return paths
}

func skillPathsFromSearchArgs(args []string) []string {
	var files []string
	patternSeen := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg != "-" && strings.HasPrefix(arg, "-") {
			flag := arg
			if before, _, ok := strings.Cut(arg, "="); ok {
				flag = before
			}
			if flag == "-e" || flag == "-f" || flag == "--regexp" || flag == "--file" {
				patternSeen = true
			}
			if searchFlagTakesValue(flag) && !strings.Contains(arg, "=") {
				i++
			}
			continue
		}
		if !patternSeen {
			patternSeen = true
			continue
		}
		files = append(files, arg)
	}
	return skillFilePaths(files)
}

func searchFlagTakesValue(flag string) bool {
	switch flag {
	case "-A", "-B", "-C", "-m", "-e", "-f", "-g", "-t", "-T",
		"--after-context", "--before-context", "--context", "--max-count",
		"--regexp", "--file", "--glob", "--type", "--type-not":
		return true
	default:
		return false
	}
}

func sedWritesInPlace(args []string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, "--in-place") {
			return true
		}
		if strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") && strings.ContainsRune(arg, 'i') {
			return true
		}
	}
	return false
}

func skillNameFromPath(path, baseDir string) string {
	path = strings.TrimSpace(path)
	if path == "" || !isSkillMarkdownPath(path) || skillPathIsGlob(path) {
		return ""
	}
	resolved, readable := resolveSkillPath(path, baseDir)
	if readable {
		if name := skillNameFromFrontmatter(resolved); name != "" {
			return name
		}
	}
	if skillPathIsBare(path) {
		return ""
	}
	return filepath.Base(filepath.Dir(resolved))
}

func resolveSkillPath(path, baseDir string) (string, bool) {
	path = expandSkillHome(path)
	if filepath.IsAbs(path) {
		return filepath.Clean(path), true
	}
	baseDir = expandSkillHome(strings.TrimSpace(baseDir))
	if filepath.IsAbs(baseDir) {
		return filepath.Clean(filepath.Join(baseDir, path)), true
	}
	return filepath.Clean(path), false
}

func expandSkillHome(path string) string {
	if path != "~" && !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}

func isSkillMarkdownPath(path string) bool {
	path = strings.ReplaceAll(path, "\\", "/")
	return path == "SKILL.md" || strings.HasSuffix(path, "/SKILL.md")
}

func skillPathIsGlob(path string) bool {
	return strings.ContainsAny(path, "*?[")
}

func skillPathIsBare(path string) bool {
	return !strings.ContainsAny(path, `/\`)
}

func skillNameFromFrontmatter(path string) string {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	f, err := openNoFollow(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	info, err = f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	b, err := io.ReadAll(io.LimitReader(f, maxSkillFrontmatterSize))
	if err != nil {
		return ""
	}
	text := strings.TrimPrefix(string(b), "\ufeff")
	if !strings.HasPrefix(text, "---") {
		return ""
	}
	for _, line := range strings.Split(text, "\n")[1:] {
		line = strings.TrimSpace(line)
		if line == "---" || line == "..." {
			return ""
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) != "name" {
			continue
		}
		return strings.Trim(strings.TrimSpace(value), `"'`)
	}
	return ""
}
