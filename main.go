package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

func credFile() string {
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		dir = filepath.Join(homeDir(), ".claude")
	}
	return filepath.Join(dir, ".credentials.json")
}

func globalFile() string {
	return filepath.Join(homeDir(), ".claude.json")
}

func storeDir() string {
	return filepath.Join(homeDir(), ".cc-accounts")
}

func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		fatalf("cannot determine home directory: %v", err)
	}
	return h
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func readJSONObject(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return m, nil
}

func writeJSONObject(path string, v any, perm os.FileMode) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

func orDefault(v any, ok bool, def any) any {
	if !ok || v == nil {
		return def
	}
	return v
}

func ensureSetup() {
	sd := storeDir()
	if err := os.MkdirAll(sd, 0700); err != nil {
		fatalf("could not create %s: %v", sd, err)
	}
	if err := os.Chmod(sd, 0700); err != nil {
		fatalf("could not chmod %s: %v", sd, err)
	}

	gf := globalFile()
	if _, err := os.Stat(gf); os.IsNotExist(err) {
		if err := os.WriteFile(gf, []byte("{}\n"), 0644); err != nil {
			fatalf("could not create %s: %v", gf, err)
		}
	}
}

func cmdSave(name string) {
	if name == "" {
		fatalf("Usage: cc-switch save <name>")
	}
	cf := credFile()
	if _, err := os.Stat(cf); os.IsNotExist(err) {
		fatalf("No credentials file at %s yet — run 'claude auth login' first.", cf)
	}

	global, err := readJSONObject(globalFile())
	if err != nil {
		fatalf("%v", err)
	}
	oauthVal, ok := global["oauthAccount"]
	oauth := orDefault(oauthVal, ok, map[string]any{})

	cred, err := readJSONObject(cf)
	if err != nil {
		fatalf("%v", err)
	}
	credVal, ok := cred["claudeAiOauth"]
	claudeAiOauth := orDefault(credVal, ok, map[string]any{})

	snap := map[string]any{
		"oauthAccount":  oauth,
		"claudeAiOauth": claudeAiOauth,
	}

	dest := filepath.Join(storeDir(), name+".json")
	if err := writeJSONObject(dest, snap, 0600); err != nil {
		fatalf("could not write %s: %v", dest, err)
	}
	fmt.Printf("Saved the currently logged-in account as '%s'.\n", name)
}

func cmdUse(name string) {
	if name == "" {
		fatalf("Usage: cc-switch use <name>")
	}
	snapPath := filepath.Join(storeDir(), name+".json")
	if _, err := os.Stat(snapPath); os.IsNotExist(err) {
		fatalf("No saved account called '%s'. Run: cc-switch save %s (while logged into it)", name, name)
	}
	snap, err := readJSONObject(snapPath)
	if err != nil {
		fatalf("%v", err)
	}
	oauth := snap["oauthAccount"]
	cred := snap["claudeAiOauth"]

	gf := globalFile()
	global, err := readJSONObject(gf)
	if err != nil {
		fatalf("%v", err)
	}
	global["oauthAccount"] = oauth
	if err := writeJSONObject(gf, global, 0600); err != nil {
		fatalf("could not write %s: %v", gf, err)
	}

	cf := credFile()
	var credFileObj map[string]any
	if _, err := os.Stat(cf); err == nil {
		credFileObj, err = readJSONObject(cf)
		if err != nil {
			fatalf("%v", err)
		}
	} else {
		credFileObj = map[string]any{}
	}
	credFileObj["claudeAiOauth"] = cred
	if err := writeJSONObject(cf, credFileObj, 0600); err != nil {
		fatalf("could not write %s: %v", cf, err)
	}

	fmt.Printf("Switched active account to '%s'\n", name)
}

func cmdList() {
	entries, err := os.ReadDir(storeDir())
	if err != nil {
		fmt.Println("(no saved accounts yet)")
		return
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".json"))
	}
	if len(names) == 0 {
		fmt.Println("(no saved accounts yet)")
		return
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Println(n)
	}
}

type usageLimit struct {
	Label    string
	Percent  float64
	ResetsAt string
	Severity string
	Active   bool
	Order    int
}

var usageKindLabels = map[string]string{
	"session":       "Session",
	"weekly_all":    "Weekly",
	"weekly_scoped": "Weekly",
}

var legacyUsageKeys = map[string]string{
	"five_hour":        "Session",
	"seven_day":        "Weekly",
	"seven_day_opus":   "Weekly (Opus)",
	"seven_day_sonnet": "Weekly (Sonnet)",
	"seven_day_cowork": "Weekly (Cowork)",
}

func parseUsageLimits(raw map[string]any) []usageLimit {
	if limits, ok := raw["limits"].([]any); ok && len(limits) > 0 {
		return parseLimitsArray(limits)
	}
	return parseLegacyUsageLimits(raw)
}

func parseLimitsArray(limits []any) []usageLimit {
	var out []usageLimit
	for i, item := range limits {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		kind, _ := m["kind"].(string)
		label := usageKindLabels[kind]
		if label == "" {
			label = humanizeUsageKey(kind)
		}
		if kind == "weekly_scoped" {
			if scope, ok := m["scope"].(map[string]any); ok {
				if model, ok := scope["model"].(map[string]any); ok {
					if name, ok := model["display_name"].(string); ok && name != "" {
						label = "Weekly (" + name + ")"
					}
				}
			}
		}
		percent, _ := asFloat64(m["percent"])
		resetsAt, _ := m["resets_at"].(string)
		severity, _ := m["severity"].(string)
		active, _ := m["is_active"].(bool)
		out = append(out, usageLimit{
			Label:    label,
			Percent:  percent,
			ResetsAt: resetsAt,
			Severity: severity,
			Active:   active,
			Order:    i,
		})
	}
	return out
}

func parseLegacyUsageLimits(raw map[string]any) []usageLimit {
	skip := map[string]bool{
		"limits": true, "extra_usage": true, "spend": true,
		"member_dashboard_available": true,
	}
	var keys []string
	for k, v := range raw {
		if skip[k] || v == nil {
			continue
		}
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if _, ok := m["utilization"]; !ok {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []usageLimit
	for i, k := range keys {
		m := raw[k].(map[string]any)
		label := legacyUsageKeys[k]
		if label == "" {
			label = humanizeUsageKey(k)
		}
		percent, _ := asFloat64(m["utilization"])
		resetsAt, _ := m["resets_at"].(string)
		out = append(out, usageLimit{
			Label:    label,
			Percent:  percent,
			ResetsAt: resetsAt,
			Order:    i,
		})
	}
	return out
}

func asFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case json.Number:
		f, err := n.Float64()
		return f, err == nil
	default:
		return 0, false
	}
}

func humanizeUsageKey(key string) string {
	if key == "" {
		return "Limit"
	}
	return strings.ReplaceAll(cases.Title(language.English).String(strings.ReplaceAll(key, "_", " ")), "Seven Day", "Weekly")
}

func formatResetAt(resetsAt string) string {
	if resetsAt == "" {
		return ""
	}
	t, err := time.Parse(time.RFC3339Nano, resetsAt)
	if err != nil {
		t, err = time.Parse(time.RFC3339, resetsAt)
		if err != nil {
			return "resets " + resetsAt
		}
	}
	now := time.Now()
	d := t.Sub(now)
	when := formatLocalTime(t)
	if d <= 0 {
		return "reset at " + when
	}
	return fmt.Sprintf("resets at %s (in %s)", when, formatDuration(d))
}

func formatLocalTime(t time.Time) string {
	local := t.Local()
	now := time.Now()
	if local.Year() == now.Year() && local.YearDay() == now.YearDay() {
		return local.Format("3:04 PM")
	}
	return local.Format("Jan 2, 3:04 PM")
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "less than a minute"
	}
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60

	var parts []string
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if mins > 0 && days == 0 {
		parts = append(parts, fmt.Sprintf("%dm", mins))
	}
	if len(parts) == 0 {
		return "less than a minute"
	}
	return strings.Join(parts, " ")
}

func printUsageLimit(limit usageLimit) {
	line := fmt.Sprintf("  %s: %.0f%% used", limit.Label, limit.Percent)
	if limit.Severity == "critical" {
		line += " (critical)"
	} else if limit.Active {
		line += " (active)"
	}
	if limit.ResetsAt != "" {
		line += " — " + formatResetAt(limit.ResetsAt)
	}
	fmt.Println(line)
}

func printAccountUsage(name string, limits []usageLimit) {
	fmt.Printf("%s:\n", name)
	if len(limits) == 0 {
		fmt.Println("  (no usage limits returned)")
		return
	}
	for _, limit := range limits {
		printUsageLimit(limit)
	}
}

func listAccountNames() []string {
	entries, err := os.ReadDir(storeDir())
	if err != nil {
		return nil
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".json"))
	}
	sort.Strings(names)
	return names
}

func accountAccessToken(name string) (string, error) {
	snapPath := filepath.Join(storeDir(), name+".json")
	snap, err := readJSONObject(snapPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("no saved account called '%s'", name)
		}
		return "", err
	}
	oauthVal, ok := snap["claudeAiOauth"]
	if !ok || oauthVal == nil {
		return "", fmt.Errorf("account '%s' has no claudeAiOauth credentials", name)
	}
	oauth, ok := oauthVal.(map[string]any)
	if !ok {
		return "", fmt.Errorf("account '%s' has invalid claudeAiOauth credentials", name)
	}
	tokenVal, ok := oauth["accessToken"]
	if !ok || tokenVal == nil {
		return "", fmt.Errorf("account '%s' has no access token", name)
	}
	token, ok := tokenVal.(string)
	if !ok || token == "" {
		return "", fmt.Errorf("account '%s' has no access token", name)
	}
	return token, nil
}

func fetchUsage(token string) ([]usageLimit, error) {
	req, err := http.NewRequest(http.MethodGet, "https://api.anthropic.com/api/oauth/usage", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("anthropic-beta", "oauth-2025-04-20")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("usage API returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parsing usage response: %w", err)
	}
	return parseUsageLimits(raw), nil
}

type accountUsageResult struct {
	name   string
	limits []usageLimit
	err    error
}

func cmdUsage(names []string) {
	if len(names) == 0 {
		names = listAccountNames()
		if len(names) == 0 {
			fmt.Println("(no saved accounts yet)")
			return
		}
	}

	results := make([]accountUsageResult, len(names))
	var wg sync.WaitGroup
	wg.Add(len(names))

	for i, name := range names {
		go func(i int, name string) {
			defer wg.Done()
			token, err := accountAccessToken(name)
			if err != nil {
				results[i] = accountUsageResult{name: name, err: err}
				return
			}
			limits, err := fetchUsage(token)
			results[i] = accountUsageResult{name: name, limits: limits, err: err}
		}(i, name)
	}

	wg.Wait()

	for i, res := range results {
		if i > 0 {
			fmt.Println()
		}
		if res.err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", res.name, res.err)
			continue
		}
		printAccountUsage(res.name, res.limits)
	}
}

func cmdWhoami() {
	global, err := readJSONObject(globalFile())
	if err != nil {
		fatalf("%v", err)
	}
	oauthVal, ok := global["oauthAccount"]
	oauth := orDefault(oauthVal, ok, "none")
	data, err := json.MarshalIndent(oauth, "", "  ")
	if err != nil {
		fatalf("%v", err)
	}
	fmt.Println(string(data))
}

func cmdHelp() {
	fmt.Print(`cc-switch — switch the active Claude Code account

Usage:
  cc-switch save <name>   snapshot the currently logged-in account
  cc-switch use <name>    switch to a saved account
  cc-switch list          list saved account names
  cc-switch whoami        show the active oauthAccount
  cc-switch usage [name]  show rate-limit usage (all accounts, or named ones)
  cc-switch help          show this help

First-time setup for multiple accounts:
  claude auth login          # login with account A
  cc-switch save personal
  claude auth logout
  claude auth login          # login with account B
  cc-switch save work
  cc-switch use personal     # switch without another browser login
  cc-switch use work
`)
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: cc-switch {save <name>|use <name>|list|whoami|usage [name...]|help}")
	fmt.Fprintln(os.Stderr, "Run 'cc-switch help' for more information.")
	os.Exit(1)
}

func arg(i int) string {
	if len(os.Args) > i {
		return os.Args[i]
	}
	return ""
}

func main() {
	ensureSetup()

	if len(os.Args) < 2 {
		usage()
	}

	switch os.Args[1] {
	case "save":
		cmdSave(arg(2))
	case "use":
		cmdUse(arg(2))
	case "list":
		cmdList()
	case "whoami":
		cmdWhoami()
	case "usage":
		cmdUsage(os.Args[2:])
	case "help", "-h", "--help":
		cmdHelp()
	default:
		usage()
	}
}
