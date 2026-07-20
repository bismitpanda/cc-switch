package main

import (
	"fmt"
	"sort"
	"strconv"
	"time"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/spf13/cobra"
)

type tokensOptions struct {
	spend      bool
	activeOnly bool
}

type accountTokenTotals struct {
	Name              string
	InputTokens       uint64
	OutputTokens      uint64
	CacheCreateTokens uint64
	CacheReadTokens   uint64
	CostUSD           float64
	Active            time.Duration
	Entries           int
}

func (t *accountTokenTotals) add(e loadedUsageEntry) {
	t.InputTokens += e.Usage.InputTokens
	t.OutputTokens += e.Usage.OutputTokens
	t.CacheCreateTokens += e.Usage.cacheCreateCount()
	t.CacheReadTokens += e.Usage.CacheReadTokens
	t.CostUSD += e.Cost
	t.Entries++
}

func (t accountTokenTotals) totalTokens() uint64 {
	return t.InputTokens + t.OutputTokens + t.CacheCreateTokens + t.CacheReadTokens
}

type accountWindow struct {
	name string
	from time.Time
}

func buildAccountTimeline(events []switchEvent) []accountWindow {
	if len(events) == 0 {
		return nil
	}
	sorted := append([]switchEvent(nil), events...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].TS < sorted[j].TS
	})

	var windows []accountWindow
	for _, ev := range sorted {
		ts, ok := parseUsageTimestamp(ev.TS)
		if !ok || ev.To == "" {
			continue
		}
		windows = append(windows, accountWindow{name: ev.To, from: ts})
	}
	return windows
}

func accountAt(windows []accountWindow, t time.Time) string {
	if len(windows) == 0 {
		return "unknown"
	}
	name := "unknown"
	for _, w := range windows {
		if !t.Before(w.from) {
			name = w.name
			continue
		}
		break
	}
	return name
}

func accountActiveDurations(windows []accountWindow, now time.Time) map[string]time.Duration {
	out := map[string]time.Duration{}
	if len(windows) == 0 {
		return out
	}
	for i, w := range windows {
		end := now
		if i+1 < len(windows) {
			end = windows[i+1].from
		}
		if end.After(w.from) {
			out[w.name] += end.Sub(w.from)
		}
	}
	return out
}

func aggregateTokensByAccount(entries []loadedUsageEntry, events []switchEvent) []accountTokenTotals {
	windows := buildAccountTimeline(events)
	durations := accountActiveDurations(windows, time.Now())
	byName := map[string]*accountTokenTotals{}
	for _, e := range entries {
		name := accountAt(windows, e.Timestamp)
		t, ok := byName[name]
		if !ok {
			t = &accountTokenTotals{Name: name}
			byName[name] = t
		}
		t.add(e)
	}
	for name, d := range durations {
		t, ok := byName[name]
		if !ok {
			t = &accountTokenTotals{Name: name}
			byName[name] = t
		}
		t.Active = d
	}
	out := make([]accountTokenTotals, 0, len(byName))
	for _, t := range byName {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].totalTokens() != out[j].totalTokens() {
			return out[i].totalTokens() > out[j].totalTokens()
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func formatTokenCount(n uint64) string {
	s := strconv.FormatUint(n, 10)
	if n < 1000 {
		return s
	}
	var b []byte
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b = append(b, ',')
		}
		b = append(b, byte(c))
	}
	return string(b)
}

func formatUSD(v float64) string {
	return fmt.Sprintf("$%.2f", v)
}

func formatActiveDuration(d time.Duration) string {
	if d <= 0 {
		return "—"
	}
	return formatDuration(d)
}

func printTokenTotals(totals []accountTokenTotals, spend bool) {
	initStyles()
	if len(totals) == 0 {
		printMuted("(no Claude Code usage entries found)")
		return
	}

	headers := []string{"Account", "Active", "Input", "Output", "Cache create", "Cache read", "Total"}
	if spend {
		headers = append(headers, "Est. spend")
	}

	rows := make([][]string, 0, len(totals)+1)
	var grand accountTokenTotals
	for _, t := range totals {
		row := []string{
			t.Name,
			formatActiveDuration(t.Active),
			formatTokenCount(t.InputTokens),
			formatTokenCount(t.OutputTokens),
			formatTokenCount(t.CacheCreateTokens),
			formatTokenCount(t.CacheReadTokens),
			formatTokenCount(t.totalTokens()),
		}
		if spend {
			row = append(row, formatUSD(t.CostUSD))
		}
		rows = append(rows, row)
		grand.InputTokens += t.InputTokens
		grand.OutputTokens += t.OutputTokens
		grand.CacheCreateTokens += t.CacheCreateTokens
		grand.CacheReadTokens += t.CacheReadTokens
		grand.CostUSD += t.CostUSD
		grand.Active += t.Active
		grand.Entries += t.Entries
	}
	totalRow := []string{
		"Total",
		formatActiveDuration(grand.Active),
		formatTokenCount(grand.InputTokens),
		formatTokenCount(grand.OutputTokens),
		formatTokenCount(grand.CacheCreateTokens),
		formatTokenCount(grand.CacheReadTokens),
		formatTokenCount(grand.totalTokens()),
	}
	if spend {
		totalRow = append(totalRow, formatUSD(grand.CostUSD))
	}
	rows = append(rows, totalRow)

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(mutedStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return labelStyle.Bold(true).Padding(0, 1)
			}
			if row == len(rows)-1 {
				return labelStyle.Bold(true).Padding(0, 1)
			}
			if col == 0 {
				return accountStyle.Padding(0, 1)
			}
			return labelStyle.Padding(0, 1)
		}).
		Headers(headers...).
		Rows(rows...)
	lipgloss.Println(t)
	if spend {
		printMuted("Estimated API-equivalent spend from local JSONL (not your subscription invoice).")
	}
}

func filterTotals(totals []accountTokenTotals, name string, activeOnly bool) []accountTokenTotals {
	if activeOnly {
		active, ok := activeSavedAccountName()
		if !ok {
			fatalf("no active saved account — run cc-switch use <name> first")
		}
		name = active
	}
	if name == "" {
		return totals
	}
	for _, t := range totals {
		if t.Name == name {
			return []accountTokenTotals{t}
		}
	}
	return nil
}

func cmdTokens(name string, opts tokensOptions) {
	entries, err := loadClaudeUsageEntries()
	if err != nil {
		fatalf("%v", err)
	}
	events, err := readSwitchLog()
	if err != nil {
		fatalf("%v", err)
	}
	totals := aggregateTokensByAccount(entries, events)
	totals = filterTotals(totals, name, opts.activeOnly)
	if name != "" && !opts.activeOnly && len(totals) == 0 {
		printMuted(fmt.Sprintf("(no usage attributed to account %q)", name))
		return
	}
	printTokenTotals(totals, opts.spend)
}

func newTokensCmd() *cobra.Command {
	var spend, activeOnly bool
	cmd := &cobra.Command{
		Use:               "tokens [name]",
		Short:             "Show Claude Code token usage by account (from local JSONL)",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeAccountNames,
		Run: func(_ *cobra.Command, args []string) {
			cmdTokens(optionalName(args), tokensOptions{
				spend:      spend,
				activeOnly: activeOnly,
			})
		},
	}
	cmd.Flags().BoolVar(&spend, "spend", false, "Include estimated USD spend")
	cmd.Flags().BoolVarP(&activeOnly, "active", "A", false, "Show only the active account")
	return cmd
}
