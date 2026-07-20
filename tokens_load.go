package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type cacheCreationBreakdown struct {
	Ephemeral5mInputTokens uint64 `json:"ephemeral_5m_input_tokens"`
	Ephemeral1hInputTokens uint64 `json:"ephemeral_1h_input_tokens"`
}

type tokenUsage struct {
	InputTokens              uint64                   `json:"input_tokens"`
	OutputTokens             uint64                   `json:"output_tokens"`
	CacheCreationInputTokens uint64                   `json:"cache_creation_input_tokens"`
	CacheReadTokens          uint64                   `json:"cache_read_input_tokens"`
	CacheCreation            *cacheCreationBreakdown  `json:"cache_creation"`
}

func (u tokenUsage) cacheCreateCount() uint64 {
	if u.CacheCreation != nil {
		return u.CacheCreation.Ephemeral5mInputTokens + u.CacheCreation.Ephemeral1hInputTokens
	}
	return u.CacheCreationInputTokens
}

func (u tokenUsage) cacheCreateSplit() (cache5m, cache1h uint64) {
	if u.CacheCreation != nil {
		return u.CacheCreation.Ephemeral5mInputTokens, u.CacheCreation.Ephemeral1hInputTokens
	}
	return u.CacheCreationInputTokens, 0
}

func (u tokenUsage) total() uint64 {
	return u.InputTokens + u.OutputTokens + u.cacheCreateCount() + u.CacheReadTokens
}

type usageMessage struct {
	ID    string     `json:"id"`
	Model string     `json:"model"`
	Usage tokenUsage `json:"usage"`
}

type usageLine struct {
	Timestamp   string       `json:"timestamp"`
	SessionID   string       `json:"sessionId"`
	RequestID   string       `json:"requestId"`
	Version     string       `json:"version"`
	CostUSD     *float64     `json:"costUSD"`
	IsSidechain *bool        `json:"isSidechain"`
	Message     usageMessage `json:"message"`
}

type loadedUsageEntry struct {
	Timestamp   time.Time
	Model       string
	MessageID   string
	RequestID   string
	IsSidechain bool
	Usage       tokenUsage
	Cost        float64
}

func claudeConfigRoots() []string {
	var roots []string
	seen := map[string]bool{}
	add := func(root string) {
		root = filepath.Clean(root)
		if root == "" || seen[root] {
			return
		}
		if st, err := os.Stat(filepath.Join(root, "projects")); err == nil && st.IsDir() {
			seen[root] = true
			roots = append(roots, root)
		}
	}

	if env := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); env != "" {
		for _, raw := range strings.Split(env, ",") {
			raw = strings.TrimSpace(raw)
			if raw == "" {
				continue
			}
			if strings.HasPrefix(raw, "~/") {
				raw = filepath.Join(homeDir(), raw[2:])
			}
			p := filepath.Clean(raw)
			if filepath.Base(p) == "projects" {
				p = filepath.Dir(p)
			}
			add(p)
		}
		if len(roots) > 0 {
			return roots
		}
	}

	xdg := os.Getenv("XDG_CONFIG_HOME")
	if xdg == "" {
		xdg = filepath.Join(homeDir(), ".config")
	}
	add(filepath.Join(xdg, "claude"))
	add(claudeDir())
	return roots
}

func collectJSONLFiles(dir string, out *[]string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if e.IsDir() {
			collectJSONLFiles(path, out)
			continue
		}
		if strings.HasSuffix(e.Name(), ".jsonl") {
			*out = append(*out, path)
		}
	}
}

func claudeUsageFiles() []string {
	var files []string
	for _, root := range claudeConfigRoots() {
		collectJSONLFiles(filepath.Join(root, "projects"), &files)
	}
	return files
}

func isSemverPrefix(v string) bool {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 3 {
		return false
	}
	for _, p := range parts[:2] {
		if p == "" || !isAllDigits(p) {
			return false
		}
	}
	return len(parts[2]) > 0 && parts[2][0] >= '0' && parts[2][0] <= '9'
}

func isValidUsageLine(data *usageLine) bool {
	if data.Version != "" && !isSemverPrefix(data.Version) {
		return false
	}
	if data.Message.ID == "" || data.Message.Model == "" {
		return false
	}
	return true
}

func parseUsageTimestamp(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, err = time.Parse(time.RFC3339, s)
		if err != nil {
			return time.Time{}, false
		}
	}
	return t, true
}

func loadClaudeUsageEntries() ([]loadedUsageEntry, error) {
	files := claudeUsageFiles()
	var all []loadedUsageEntry
	for _, file := range files {
		entries, err := readClaudeUsageFile(file)
		if err != nil {
			continue
		}
		all = append(all, entries...)
	}
	return dedupeUsageEntries(all), nil
}

func readClaudeUsageFile(path string) ([]loadedUsageEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var out []loadedUsageEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	marker := []byte(`"usage":{`)
	for sc.Scan() {
		line := sc.Bytes()
		if !bytes.Contains(line, marker) {
			continue
		}
		var data usageLine
		if err := json.Unmarshal(line, &data); err != nil {
			continue
		}
		ts, ok := parseUsageTimestamp(data.Timestamp)
		if !ok {
			continue
		}
		if !isValidUsageLine(&data) {
			continue
		}
		sidechain := data.IsSidechain != nil && *data.IsSidechain
		cost := resolveEntryCost(data.Message.Model, data.Message.Usage, data.CostUSD)
		out = append(out, loadedUsageEntry{
			Timestamp:   ts,
			Model:       data.Message.Model,
			MessageID:   data.Message.ID,
			RequestID:   data.RequestID,
			IsSidechain: sidechain,
			Usage:       data.Message.Usage,
			Cost:        cost,
		})
	}
	return out, sc.Err()
}

func usageDedupeKey(messageID, requestID string) string {
	return messageID + "\x00" + requestID
}

func shouldReplaceUsage(candidate, existing loadedUsageEntry) bool {
	if candidate.IsSidechain != existing.IsSidechain {
		return existing.IsSidechain
	}
	ct, et := candidate.Usage.total(), existing.Usage.total()
	if ct != et {
		return ct > et
	}
	return false
}

func dedupeUsageEntries(entries []loadedUsageEntry) []loadedUsageEntry {
	byExact := map[string]int{}
	byMessage := map[string][]int{}
	var out []loadedUsageEntry

	for _, entry := range entries {
		if entry.MessageID == "" {
			out = append(out, entry)
			continue
		}
		exact := usageDedupeKey(entry.MessageID, entry.RequestID)
		if idx, ok := byExact[exact]; ok {
			if shouldReplaceUsage(entry, out[idx]) {
				out[idx] = entry
			}
			continue
		}
		replaced := false
		for _, idx := range byMessage[entry.MessageID] {
			existing := out[idx]
			if entry.IsSidechain || existing.IsSidechain {
				if shouldReplaceUsage(entry, existing) {
					out[idx] = entry
					byExact[usageDedupeKey(entry.MessageID, entry.RequestID)] = idx
				}
				replaced = true
				break
			}
		}
		if replaced {
			continue
		}
		idx := len(out)
		out = append(out, entry)
		byExact[exact] = idx
		byMessage[entry.MessageID] = append(byMessage[entry.MessageID], idx)
	}
	return out
}
