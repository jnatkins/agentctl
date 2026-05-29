package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jnatkins/agentctl/internal/catalog"
	"github.com/jnatkins/agentctl/internal/pathutil"
	"github.com/jnatkins/agentctl/internal/target"
)

type Options struct {
	Selector  target.Selector
	OutputDir string
	Install   bool
}

type File struct {
	Path    string `json:"path"`
	Mode    uint32 `json:"mode"`
	Reason  string `json:"reason"`
	Content []byte `json:"-"`
}

func All(c *catalog.Catalog, opts Options) ([]File, error) {
	var files []File
	codexFiles, err := Codex(c, opts)
	if err != nil {
		return nil, err
	}
	files = append(files, codexFiles...)
	claudeFiles, err := Claude(c, opts)
	if err != nil {
		return nil, err
	}
	files = append(files, claudeFiles...)
	launchdFiles, err := Launchd(c, opts)
	if err != nil {
		return nil, err
	}
	files = append(files, launchdFiles...)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func Codex(c *catalog.Catalog, opts Options) ([]File, error) {
	var files []File
	for _, w := range c.AgentWorkloads {
		if !isActive(w.Status) || w.Kind != "schedule" || !target.Matches(w.Targets, c, opts.Selector) || !contains(w.Harnesses, "codex") {
			continue
		}
		path := filepath.Join(opts.OutputDir, "codex", "automations", w.ID, "automation.toml")
		if opts.Install {
			path = filepath.Join(codexHome(), "automations", w.ID, "automation.toml")
		}
		content, err := codexAutomation(w, path)
		if err != nil {
			return nil, err
		}
		files = append(files, File{Path: path, Mode: 0o644, Reason: "Codex scheduled workload", Content: content})
	}
	return files, nil
}

func codexAutomation(w catalog.AgentWorkload, existingPath string) ([]byte, error) {
	prompt, err := workloadPrompt(w)
	if err != nil {
		return nil, err
	}
	createdAt, updatedAt := existingTimestamps(existingPath)
	var b strings.Builder
	writeIntKV(&b, "version", 1)
	writeKV(&b, "id", w.ID)
	writeKV(&b, "kind", defaultString(w.CodexKind, "cron"))
	writeKV(&b, "name", w.DisplayName())
	writeKV(&b, "prompt", prompt)
	writeKV(&b, "status", strings.ToUpper(defaultString(w.Status, "active")))
	writeKV(&b, "rrule", w.Schedule)
	if w.Model != "" {
		writeKV(&b, "model", w.Model)
	}
	if w.Reasoning != "" {
		writeKV(&b, "reasoning_effort", w.Reasoning)
	}
	if w.ExecutionEnvironment != "" {
		writeKV(&b, "execution_environment", w.ExecutionEnvironment)
	}
	if w.CWD != "" {
		writeArrayKV(&b, "cwds", []string{pathutil.Expand(w.CWD)})
	}
	if w.TargetThreadID != "" {
		writeKV(&b, "target_thread_id", w.TargetThreadID)
	}
	writeIntKV(&b, "created_at", createdAt)
	writeIntKV(&b, "updated_at", updatedAt)
	return []byte(b.String()), nil
}

func Claude(c *catalog.Catalog, opts Options) ([]File, error) {
	servers := make(map[string]map[string]any)
	allowed := make([]string, 0)
	for _, in := range c.Integrations {
		if !isActive(in.Status) || !target.Matches(in.Targets, c, opts.Selector) || !contains(in.Harnesses, "claude") || !strings.HasPrefix(in.Type, "mcp_") {
			continue
		}
		server := make(map[string]any)
		switch in.Type {
		case "mcp_stdio":
			server["command"] = in.Command
			if len(in.Args) > 0 {
				server["args"] = in.Args
			}
		case "mcp_http", "mcp_hosted_connector":
			server["type"] = "http"
			if in.URL != "" {
				server["url"] = in.URL
			}
		case "mcp_sse":
			server["type"] = "sse"
			if in.URL != "" {
				server["url"] = in.URL
			}
		}
		if len(in.EnvRefs) > 0 {
			server["env"] = in.EnvRefs
		}
		if len(in.Headers) > 0 {
			server["headers"] = in.Headers
		}
		servers[in.ID] = server
		allowed = append(allowed, in.AllowedTools...)
	}
	if len(servers) == 0 {
		return nil, nil
	}
	sort.Strings(allowed)
	doc := map[string]any{
		"mcpServers": servers,
	}
	if len(allowed) > 0 {
		doc["allowedTools"] = allowed
	}
	content, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	content = append(content, '\n')
	path := filepath.Join(opts.OutputDir, "claude", "agentctl.mcp.json")
	if opts.Install {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, ".claude", "agentctl.mcp.json")
		}
	}
	return []File{{Path: path, Mode: 0o644, Reason: "Claude MCP integration config", Content: content}}, nil
}

func Launchd(c *catalog.Catalog, opts Options) ([]File, error) {
	var files []File
	for _, w := range c.AgentWorkloads {
		if !isActive(w.Status) || !target.Matches(w.Targets, c, opts.Selector) || (w.Kind != "daemon" && w.Kind != "queue") {
			continue
		}
		content := launchdPlist("dev.agentctl.workload."+slug(w.ID), w.DisplayName(), w.Command, w.Args, w.CWD, w.LogPath, w.RestartPolicy, "")
		files = append(files, File{
			Path:    renderPath(opts, "launchd", "dev.agentctl.workload."+slug(w.ID)+".plist"),
			Mode:    0o644,
			Reason:  "launchd agent workload wrapper",
			Content: content,
		})
	}
	for _, svc := range c.AuxServices {
		if !isActive(svc.Status) || !target.Matches(svc.Targets, c, opts.Selector) || svc.Command == "" {
			continue
		}
		content := launchdPlist("dev.agentctl.service."+slug(svc.ID), svc.ID, svc.Command, svc.Args, svc.CWD, svc.LogPath, svc.RestartPolicy, svc.Schedule)
		files = append(files, File{
			Path:    renderPath(opts, "launchd", "dev.agentctl.service."+slug(svc.ID)+".plist"),
			Mode:    0o644,
			Reason:  "launchd auxiliary service wrapper",
			Content: content,
		})
	}
	return files, nil
}

func renderPath(opts Options, dir, name string) string {
	if opts.Install {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, "Library", "LaunchAgents", name)
		}
	}
	return filepath.Join(opts.OutputDir, dir, name)
}

func launchdPlist(label, name, command string, args []string, cwd, logPath, restart, schedule string) []byte {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString(`<plist version="1.0">` + "\n")
	b.WriteString("<dict>\n")
	writePlistKeyString(&b, "Label", label)
	writePlistKeyArray(&b, "ProgramArguments", append([]string{pathutil.Expand(command)}, args...))
	if cwd != "" {
		writePlistKeyString(&b, "WorkingDirectory", pathutil.Expand(cwd))
	}
	if interval, ok := parseStartInterval(schedule); ok {
		// Interval-scheduled service: run every N seconds, do not keep alive.
		writePlistKeyInteger(&b, "StartInterval", interval)
		writePlistKeyBool(&b, "KeepAlive", false)
	} else {
		writePlistKeyBool(&b, "KeepAlive", restart == "always" || restart == "")
	}
	writePlistKeyBool(&b, "RunAtLoad", true)
	if logPath != "" {
		expanded := pathutil.Expand(logPath)
		writePlistKeyString(&b, "StandardOutPath", expanded)
		writePlistKeyString(&b, "StandardErrorPath", expanded)
	}
	_ = name
	b.WriteString("</dict>\n</plist>\n")
	return b.Bytes()
}

// parseStartInterval extracts the seconds from a "StartInterval=<n>" schedule.
// Returns false for any other (or empty) schedule form.
func parseStartInterval(schedule string) (int, bool) {
	const prefix = "StartInterval="
	if !strings.HasPrefix(schedule, prefix) {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(schedule[len(prefix):]))
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

func writePlistKeyInteger(b *bytes.Buffer, key string, value int) {
	fmt.Fprintf(b, "  <key>%s</key>\n  <integer>%d</integer>\n", xmlEscape(key), value)
}

func workloadPrompt(w catalog.AgentWorkload) (string, error) {
	if w.Prompt != "" {
		return strings.TrimRight(w.Prompt, "\r\n"), nil
	}
	if w.PromptFile == "" {
		return "", nil
	}
	path := pathutil.ResolveRelative(w.SourceDir(), w.PromptFile)
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("%s: read prompt_file %s: %w", w.ID, path, err)
	}
	return strings.TrimRight(string(content), "\r\n"), nil
}

func writeKV(b *strings.Builder, key, value string) {
	b.WriteString(key)
	b.WriteString(" = ")
	b.WriteString(strconv.Quote(value))
	b.WriteByte('\n')
}

func writeIntKV(b *strings.Builder, key string, value int64) {
	b.WriteString(key)
	b.WriteString(" = ")
	b.WriteString(strconv.FormatInt(value, 10))
	b.WriteByte('\n')
}

func writeArrayKV(b *strings.Builder, key string, values []string) {
	b.WriteString(key)
	b.WriteString(" = [")
	for i, value := range values {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strconv.Quote(value))
	}
	b.WriteString("]\n")
}

func writePlistKeyString(b *bytes.Buffer, key, value string) {
	fmt.Fprintf(b, "  <key>%s</key>\n  <string>%s</string>\n", xmlEscape(key), xmlEscape(value))
}

func writePlistKeyBool(b *bytes.Buffer, key string, value bool) {
	fmt.Fprintf(b, "  <key>%s</key>\n", xmlEscape(key))
	if value {
		b.WriteString("  <true/>\n")
	} else {
		b.WriteString("  <false/>\n")
	}
}

func writePlistKeyArray(b *bytes.Buffer, key string, values []string) {
	fmt.Fprintf(b, "  <key>%s</key>\n  <array>\n", xmlEscape(key))
	for _, value := range values {
		fmt.Fprintf(b, "    <string>%s</string>\n", xmlEscape(value))
	}
	b.WriteString("  </array>\n")
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return r.Replace(s)
}

func codexHome() string {
	if v := os.Getenv("CODEX_HOME"); v != "" {
		return pathutil.Expand(v)
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".codex")
	}
	return ".codex"
}

func defaultString(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}

func isActive(status string) bool {
	return status == "" || status == "active"
}

func existingTimestamps(path string) (int64, int64) {
	now := time.Now().UnixMilli()
	data, err := os.ReadFile(path)
	if err != nil {
		return now, now
	}
	created := parseIntField(data, "created_at", now)
	updated := parseIntField(data, "updated_at", created)
	return created, updated
}

func parseIntField(data []byte, key string, fallback int64) int64 {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + ` = ([0-9]+)$`)
	match := re.FindSubmatch(data)
	if len(match) != 2 {
		return fallback
	}
	value, err := strconv.ParseInt(string(match[1]), 10, 64)
	if err != nil {
		return fallback
	}
	return value
}

var slugPattern = regexp.MustCompile(`[^a-zA-Z0-9.-]+`)

func slug(value string) string {
	value = slugPattern.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}

func contains(values []string, needle string) bool {
	for _, v := range values {
		if v == needle {
			return true
		}
	}
	return false
}
