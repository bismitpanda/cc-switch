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

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/muesli/termenv"
	"golang.org/x/term"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

var (
	stylesReady bool

	titleStyle       lipgloss.Style
	successStyle     lipgloss.Style
	mutedStyle       lipgloss.Style
	errorStyle       lipgloss.Style
	accountStyle     lipgloss.Style
	labelStyle       lipgloss.Style
	barFillStyle     lipgloss.Style
	barEmptyStyle    lipgloss.Style
	criticalBarStyle lipgloss.Style
	activeBarStyle   lipgloss.Style
	whoamiKeyStyle   lipgloss.Style
	whoamiValStyle   lipgloss.Style
	helpTitleStyle   lipgloss.Style
	helpCmdStyle     lipgloss.Style
)

func initStyles() {
	if stylesReady {
		return
	}
	stylesReady = true

	if !term.IsTerminal(int(os.Stdout.Fd())) {
		lipgloss.SetColorProfile(termenv.Ascii)
	}

	titleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	successStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	mutedStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	errorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	accountStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("117"))
	labelStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	barFillStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	barEmptyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	criticalBarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	activeBarStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	whoamiKeyStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	whoamiValStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	helpTitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	helpCmdStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
}

func printMuted(msg string) {
	initStyles()
	fmt.Println(mutedStyle.Render(msg))
}

func printSuccess(msg string) {
	initStyles()
	fmt.Println(successStyle.Render(msg))
}

func printError(name, msg string) {
	initStyles()
	if name != "" {
		fmt.Fprintf(os.Stderr, "%s: %s\n", accountStyle.Render(name), errorStyle.Render(msg))
		return
	}
	fmt.Fprintln(os.Stderr, errorStyle.Render(msg))
}

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
		if !isInteractive() {
			fatalf("Usage: cc-switch save <name>")
		}
		name = promptSaveName()
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
	printSuccess(fmt.Sprintf("Saved the currently logged-in account as %s.", accountStyle.Render(name)))
}

func cmdUse(name string) {
	if name == "" {
		if !isInteractive() {
			fatalf("Usage: cc-switch use <name>")
		}
		name = promptUseAccount()
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

	printSuccess(fmt.Sprintf("Switched active account to %s", accountStyle.Render(name)))
}

func oauthFieldPlain(oauth map[string]any, key string) string {
	value, ok := whoamiFieldValue(oauth[key])
	if !ok {
		return "—"
	}
	return formatWhoamiField(key, value)
}

func savedOAuthAccount(name string) (map[string]any, bool) {
	snap, err := readJSONObject(filepath.Join(storeDir(), name+".json"))
	if err != nil {
		return nil, false
	}
	oauthVal, ok := snap["oauthAccount"]
	if !ok || oauthVal == nil {
		return nil, false
	}
	m, ok := oauthVal.(map[string]any)
	if !ok || len(m) == 0 {
		return nil, false
	}
	return m, true
}

func cmdList() {
	initStyles()
	entries, err := os.ReadDir(storeDir())
	if err != nil {
		printMuted("(no saved accounts yet)")
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
		printMuted("(no saved accounts yet)")
		return
	}
	sort.Strings(names)

	active, _ := activeOAuthAccount()
	activeRows := make([]bool, len(names))
	rows := make([][]string, len(names))
	for i, name := range names {
		activeRows[i] = isActiveSavedAccount(name, active)
		account := name
		if activeRows[i] {
			account = "● " + name
		}
		email, org, typ := "—", "—", "—"
		if oauth, ok := savedOAuthAccount(name); ok {
			email = oauthFieldPlain(oauth, "emailAddress")
			org = oauthFieldPlain(oauth, "organizationName")
			typ = oauthFieldPlain(oauth, "organizationType")
		}
		rows[i] = []string{account, email, org, typ}
	}

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(mutedStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return labelStyle.Bold(true)
			}
			if activeRows[row] && col == 0 {
				return accountStyle.Bold(true).Foreground(lipgloss.Color("42"))
			}
			if col == 0 {
				return accountStyle
			}
			return whoamiValStyle
		}).
		Headers("Account", "Email", "Organization", "Type").
		Rows(rows...)
	fmt.Println(t)
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

const (
	usageBarWidth  = 20
	usagePctWidth  = 4
	usageMinLabelW = 16
)

func usageBar(percent float64, severity string, active bool) string {
	filled := int(percent / 100 * usageBarWidth)
	if filled > usageBarWidth {
		filled = usageBarWidth
	}
	if filled < 0 {
		filled = 0
	}
	empty := usageBarWidth - filled

	fillStyle := barFillStyle
	switch {
	case severity == "critical":
		fillStyle = criticalBarStyle
	case active:
		fillStyle = activeBarStyle
	}

	return fillStyle.Render(strings.Repeat("█", filled)) +
		barEmptyStyle.Render(strings.Repeat("░", empty))
}

func usageLabelWidth(limits []usageLimit) int {
	width := usageMinLabelW
	for _, limit := range limits {
		if w := lipgloss.Width(limit.Label); w > width {
			width = w
		}
	}
	return width
}

func printUsageLimit(limit usageLimit, labelWidth int) {
	initStyles()
	bar := usageBar(limit.Percent, limit.Severity, limit.Active)
	pct := fmt.Sprintf("%3.0f%%", limit.Percent)
	line := lipgloss.JoinHorizontal(lipgloss.Top,
		"  ",
		labelStyle.Width(labelWidth).Render(limit.Label),
		"  ",
		lipgloss.NewStyle().Width(usageBarWidth).Render(bar),
		" ",
		mutedStyle.Width(usagePctWidth).Align(lipgloss.Right).Render(pct),
	)
	if limit.ResetsAt != "" {
		line += " " + mutedStyle.Render("— "+formatResetAt(limit.ResetsAt))
	}
	fmt.Println(line)
}

func printAccountUsage(name string, limits []usageLimit, active bool) {
	initStyles()
	header := titleStyle.Render(name)
	if active {
		header += " " + successStyle.Render("● active")
	}
	fmt.Println(header)
	if len(limits) == 0 {
		printMuted("  (no usage limits returned)")
		return
	}
	labelWidth := usageLabelWidth(limits)
	for _, limit := range limits {
		printUsageLimit(limit, labelWidth)
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

func activeOAuthAccount() (any, error) {
	global, err := readJSONObject(globalFile())
	if err != nil {
		return nil, err
	}
	oauthVal, ok := global["oauthAccount"]
	return orDefault(oauthVal, ok, nil), nil
}

func jsonEqual(a, b any) bool {
	da, err := json.Marshal(a)
	if err != nil {
		return false
	}
	db, err := json.Marshal(b)
	if err != nil {
		return false
	}
	return string(da) == string(db)
}

func isActiveSavedAccount(name string, active any) bool {
	if active == nil {
		return false
	}
	snap, err := readJSONObject(filepath.Join(storeDir(), name+".json"))
	if err != nil {
		return false
	}
	saved, ok := snap["oauthAccount"]
	if !ok || saved == nil {
		return false
	}
	return jsonEqual(active, saved)
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
			printMuted("(no saved accounts yet)")
			return
		}
	}

	results := make([]accountUsageResult, len(names))
	var wg sync.WaitGroup
	wg.Add(len(names))

	fetch := func() {
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
	}

	label := "Fetching usage..."
	if len(names) == 1 {
		label = fmt.Sprintf("Fetching usage for %s...", names[0])
	} else {
		label = fmt.Sprintf("Fetching usage for %d accounts...", len(names))
	}
	runWithLoader(label, fetch)

	active, _ := activeOAuthAccount()

	for i, res := range results {
		if i > 0 {
			fmt.Println()
		}
		if res.err != nil {
			printError(res.name, res.err.Error())
			continue
		}
		printAccountUsage(res.name, res.limits, isActiveSavedAccount(res.name, active))
	}
}

var whoamiFields = []struct {
	key   string
	label string
}{
	{"displayName", "Name"},
	{"emailAddress", "Email"},
	{"organizationName", "Organization"},
	{"organizationType", "Type"},
}

func whoamiFieldValue(v any) (string, bool) {
	if v == nil {
		return "", false
	}
	switch val := v.(type) {
	case string:
		s := strings.TrimSpace(val)
		return s, s != ""
	default:
		return "", false
	}
}

func formatWhoamiField(key, value string) string {
	if key == "organizationType" || key == "billingType" {
		return cases.Title(language.English).String(strings.ReplaceAll(value, "_", " "))
	}
	return value
}

func printWhoami(oauth any) {
	initStyles()
	if oauth == nil {
		printMuted("(no active account)")
		return
	}
	m, ok := oauth.(map[string]any)
	if !ok || len(m) == 0 {
		printMuted("(no active account)")
		return
	}

	var lines []struct {
		label string
		value string
	}
	for _, field := range whoamiFields {
		value, ok := whoamiFieldValue(m[field.key])
		if !ok {
			continue
		}
		lines = append(lines, struct {
			label string
			value string
		}{field.label, formatWhoamiField(field.key, value)})
	}
	if len(lines) == 0 {
		printMuted("(no active account)")
		return
	}

	labelWidth := 0
	for _, line := range lines {
		if w := lipgloss.Width(line.label + ":"); w > labelWidth {
			labelWidth = w
		}
	}

	fmt.Println(titleStyle.Render("Active account"))
	for _, line := range lines {
		fmt.Printf("  %s %s\n",
			whoamiKeyStyle.Width(labelWidth).Align(lipgloss.Right).Render(line.label+":"),
			whoamiValStyle.Render(line.value),
		)
	}
}

func cmdWhoami() {
	oauth, err := activeOAuthAccount()
	if err != nil {
		fatalf("%v", err)
	}
	printWhoami(oauth)
}

func cmdHelp() {
	initStyles()
	fmt.Println(helpTitleStyle.Render("cc-switch — switch the active Claude Code account"))
	fmt.Println()
	fmt.Println(labelStyle.Render("Usage:"))
	helpLines := []struct {
		cmd  string
		desc string
	}{
		{"cc-switch save [name]", "snapshot the currently logged-in account"},
		{"cc-switch use [name]", "switch to a saved account"},
		{"cc-switch list", "list saved accounts with details"},
		{"cc-switch whoami", "show the active account"},
		{"cc-switch usage [name]", "show rate-limit usage (all accounts, or named ones)"},
		{"cc-switch help", "show this help"},
	}
	for _, line := range helpLines {
		fmt.Printf("  %s  %s\n", helpCmdStyle.Render(line.cmd), mutedStyle.Render(line.desc))
	}
	fmt.Println()
	fmt.Println(labelStyle.Render("First-time setup for multiple accounts:"))
	setupLines := []string{
		"claude auth login          # login with account A",
		"cc-switch save personal",
		"claude auth logout",
		"claude auth login          # login with account B",
		"cc-switch save work",
		"cc-switch use personal     # switch without another browser login",
		"cc-switch use work",
	}
	for _, line := range setupLines {
		fmt.Println("  " + mutedStyle.Render(line))
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: cc-switch {save [name]|use [name]|list|whoami|usage [name...]|help}")
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
	initStyles()
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
