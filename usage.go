package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

type usageLimit struct {
	Kind     string
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

func coreUsageKind(kind string) string {
	switch kind {
	case "session", "five_hour":
		return "session"
	case "weekly_all", "seven_day":
		return "weekly"
	default:
		return ""
	}
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
			Kind:     coreUsageKind(kind),
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
			Kind:     coreUsageKind(k),
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

func printAccountUsage(name string, limits []usageLimit, active, grayed bool) {
	initStyles()
	if grayed {
		line := mutedStyle.Render(name)
		if resetAt, ok := usableAgainResetsAt(limits); ok {
			line += " " + mutedStyle.Render("— "+formatResetAt(resetAt))
		}
		fmt.Println(line)
	} else {
		header := titleStyle.Render(name)
		if active {
			header += " " + successStyle.Render("● active")
		}
		fmt.Println(header)
	}
	if len(limits) == 0 {
		printMuted("  (no usage limits returned)")
		return
	}
	labelWidth := usageLabelWidth(limits)
	for _, limit := range limits {
		printUsageLimit(limit, labelWidth)
	}
}

func coreUsageExhausted(limits []usageLimit) bool {
	for _, limit := range limits {
		if (limit.Kind == "session" || limit.Kind == "weekly") && limit.Percent >= 100 {
			return true
		}
	}
	return false
}

func parseResetsAt(resetsAt string) (time.Time, bool) {
	if resetsAt == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339Nano, resetsAt)
	if err != nil {
		t, err = time.Parse(time.RFC3339, resetsAt)
		if err != nil {
			return time.Time{}, false
		}
	}
	return t, true
}

func usableAgainAt(limits []usageLimit) (time.Time, bool) {
	var latest time.Time
	found := false
	for _, limit := range limits {
		if (limit.Kind != "session" && limit.Kind != "weekly") || limit.Percent < 100 {
			continue
		}
		t, ok := parseResetsAt(limit.ResetsAt)
		if !ok {
			continue
		}
		if !found || t.After(latest) {
			latest = t
			found = true
		}
	}
	return latest, found
}

func usableAgainResetsAt(limits []usageLimit) (string, bool) {
	var best string
	var latest time.Time
	found := false
	for _, limit := range limits {
		if (limit.Kind != "session" && limit.Kind != "weekly") || limit.Percent < 100 {
			continue
		}
		t, ok := parseResetsAt(limit.ResetsAt)
		if !ok {
			continue
		}
		if !found || t.After(latest) {
			latest = t
			best = limit.ResetsAt
			found = true
		}
	}
	return best, found
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

func sortUnavailableByReset(results []accountUsageResult) {
	sort.SliceStable(results, func(i, j int) bool {
		a, b := results[i], results[j]
		if a.err != nil && b.err == nil {
			return false
		}
		if a.err == nil && b.err != nil {
			return true
		}
		ti, oki := usableAgainAt(a.limits)
		tj, okj := usableAgainAt(b.limits)
		if oki != okj {
			return oki
		}
		if oki && !ti.Equal(tj) {
			return ti.Before(tj)
		}
		return a.name < b.name
	})
}

func coreLimitPercent(limits []usageLimit, kind string) (float64, bool) {
	for _, limit := range limits {
		if limit.Kind == kind {
			return limit.Percent, true
		}
	}
	return 0, false
}

func sortUsableByUsage(results []accountUsageResult) {
	sort.SliceStable(results, func(i, j int) bool {
		a, b := results[i], results[j]
		sa, soa := coreLimitPercent(a.limits, "session")
		sb, sob := coreLimitPercent(b.limits, "session")
		if soa != sob {
			return soa
		}
		if soa && sa != sb {
			return sa < sb
		}
		wa, woa := coreLimitPercent(a.limits, "weekly")
		wb, wob := coreLimitPercent(b.limits, "weekly")
		if woa != wob {
			return woa
		}
		if woa && wa != wb {
			return wa < wb
		}
		return a.name < b.name
	})
}

func printUnavailableAccounts(results []accountUsageResult) {
	initStyles()
	sortUnavailableByReset(results)
	for i, res := range results {
		if i > 0 {
			fmt.Println()
		}
		if res.err != nil {
			fmt.Println(mutedStyle.Render(res.name))
			printMuted("  " + res.err.Error())
			continue
		}
		printAccountUsage(res.name, res.limits, false, true)
	}
}

func cmdUsage(names []string) {
	if len(names) == 0 {
		names = listAccountNames()
		if len(names) == 0 {
			printMuted("(no saved accounts yet)")
			return
		}
	} else {
		for i, name := range names {
			names[i] = requireAccountName(name)
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

	var activeRes *accountUsageResult
	var usable, unavailable []accountUsageResult
	for i := range results {
		res := results[i]
		if isActiveSavedAccount(res.name, active) {
			activeRes = &results[i]
			continue
		}
		if res.err != nil || coreUsageExhausted(res.limits) {
			unavailable = append(unavailable, res)
			continue
		}
		usable = append(usable, res)
	}

	printed := false
	if activeRes != nil {
		if activeRes.err != nil {
			printError(activeRes.name, activeRes.err.Error())
		} else {
			printAccountUsage(activeRes.name, activeRes.limits, true, false)
		}
		printed = true
	}
	sortUsableByUsage(usable)
	for _, res := range usable {
		if printed {
			fmt.Println()
		}
		printAccountUsage(res.name, res.limits, false, false)
		printed = true
	}
	if len(unavailable) > 0 {
		if printed {
			fmt.Println()
		}
		printUnavailableAccounts(unavailable)
	}
}
