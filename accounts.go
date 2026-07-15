package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
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
