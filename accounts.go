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

func claudeDir() string {
	dir := os.Getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		dir = filepath.Join(homeDir(), ".claude")
	}
	return dir
}

func credFile() string {
	return filepath.Join(claudeDir(), ".credentials.json")
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

func accountSnapPath(name string) string {
	return filepath.Join(storeDir(), requireAccountName(name)+".json")
}

func resolveAccountName(name string, usage string) string {
	if name == "" {
		if !isInteractive() {
			fatalf("Usage: %s", usage)
		}
		return promptSelectAccount()
	}
	return requireAccountName(name)
}

func liveCredentials() (oauth any, claudeAiOauth any, err error) {
	cf := credFile()
	if _, err := os.Stat(cf); os.IsNotExist(err) {
		return nil, nil, fmt.Errorf("no credentials file at %s yet — run 'claude auth login' first", cf)
	}

	global, err := readJSONObject(globalFile())
	if err != nil {
		return nil, nil, err
	}
	oauthVal, ok := global["oauthAccount"]
	oauth = orDefault(oauthVal, ok, map[string]any{})

	cred, err := readJSONObject(cf)
	if err != nil {
		return nil, nil, err
	}
	credVal, ok := cred["claudeAiOauth"]
	claudeAiOauth = orDefault(credVal, ok, map[string]any{})
	return oauth, claudeAiOauth, nil
}

func writeAccountSnapshot(name string, oauth, claudeAiOauth any) error {
	snap := map[string]any{
		"oauthAccount":  oauth,
		"claudeAiOauth": claudeAiOauth,
	}
	return writeJSONObject(accountSnapPath(name), snap, 0600)
}

func activeSavedAccountName() (string, bool) {
	active, err := activeOAuthAccount()
	if err != nil || active == nil {
		return "", false
	}
	for _, name := range listAccountNames() {
		if isActiveSavedAccount(name, active) {
			return name, true
		}
	}
	return "", false
}

func syncActiveSnapshot() (string, error) {
	name, ok := activeSavedAccountName()
	if !ok {
		return "", fmt.Errorf("active account is not saved — run: cc-switch save <name>")
	}
	oauth, claudeAiOauth, err := liveCredentials()
	if err != nil {
		return "", err
	}
	if err := writeAccountSnapshot(name, oauth, claudeAiOauth); err != nil {
		return "", fmt.Errorf("could not write %s: %w", accountSnapPath(name), err)
	}
	return name, nil
}

func cmdSave(name string) {
	if name == "" {
		if !isInteractive() {
			fatalf("Usage: cc-switch save <name>")
		}
		name = promptSaveName()
	} else {
		name = requireAccountName(name)
	}

	oauth, claudeAiOauth, err := liveCredentials()
	if err != nil {
		fatalf("%v", err)
	}
	if err := writeAccountSnapshot(name, oauth, claudeAiOauth); err != nil {
		fatalf("could not write %s: %v", accountSnapPath(name), err)
	}
	printSuccess(fmt.Sprintf("Saved the currently logged-in account as %s.", accountStyle.Render(name)))
}

func cmdSync() {
	name, err := syncActiveSnapshot()
	if err != nil {
		fatalf("%v", err)
	}
	printSuccess(fmt.Sprintf("Synced live credentials into %s.", accountStyle.Render(name)))
}

func cmdUse(name string) {
	name = resolveAccountName(name, "cc-switch use <name>")
	snapPath := accountSnapPath(name)
	if _, err := os.Stat(snapPath); os.IsNotExist(err) {
		fatalf("No saved account called '%s'. Run: cc-switch save %s (while logged into it)", name, name)
	}

	if current, ok := activeSavedAccountName(); ok && current != name {
		if _, err := syncActiveSnapshot(); err != nil {
			fatalf("%v", err)
		}
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

func cmdRemove(name string) {
	name = resolveAccountName(name, "cc-switch remove <name>")
	snapPath := accountSnapPath(name)
	if err := os.Remove(snapPath); err != nil {
		if os.IsNotExist(err) {
			fatalf("No saved account called '%s'", name)
		}
		fatalf("could not remove %s: %v", name, err)
	}
	printSuccess(fmt.Sprintf("Removed saved account %s.", accountStyle.Render(name)))
}

func cmdRename(oldName, newName string) {
	if oldName == "" {
		if !isInteractive() {
			fatalf("Usage: cc-switch rename <old> <new>")
		}
		oldName = promptSelectAccount()
	} else {
		oldName = requireAccountName(oldName)
	}
	if newName == "" {
		if !isInteractive() {
			fatalf("Usage: cc-switch rename <old> <new>")
		}
		newName = promptAccountName("work")
	} else {
		newName = requireAccountName(newName)
	}
	if oldName == newName {
		fatalf("old and new names are the same")
	}

	oldPath := accountSnapPath(oldName)
	newPath := accountSnapPath(newName)
	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		fatalf("No saved account called '%s'", oldName)
	}
	if _, err := os.Stat(newPath); err == nil {
		fatalf("An account named '%s' already exists", newName)
	} else if !os.IsNotExist(err) {
		fatalf("could not check %s: %v", newName, err)
	}

	if err := os.Rename(oldPath, newPath); err != nil {
		fatalf("could not rename %s to %s: %v", oldName, newName, err)
	}
	printSuccess(fmt.Sprintf("Renamed %s → %s.", accountStyle.Render(oldName), accountStyle.Render(newName)))
}

func oauthFieldPlain(oauth map[string]any, key string) string {
	value, ok := whoamiFieldValue(oauth[key])
	if !ok {
		return "—"
	}
	return formatWhoamiField(key, value)
}

func savedOAuthAccount(name string) (map[string]any, bool) {
	if err := validateAccountName(name); err != nil {
		return nil, false
	}
	snap, err := readJSONObject(accountSnapPath(name))
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
	names := listAccountNames()
	if len(names) == 0 {
		printMuted("(no saved accounts yet)")
		return
	}

	active, _ := activeOAuthAccount()
	activeRows := make([]bool, len(names))
	rows := make([][]string, len(names))
	for i, name := range names {
		activeRows[i] = isActiveSavedAccount(name, active)
		account := name
		if activeRows[i] {
			account = name + " ●"
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
				return labelStyle.Bold(true).Padding(0, 1)
			}
			if activeRows[row] && col == 0 {
				return accountStyle.Bold(true).Foreground(lipgloss.Color("42")).Padding(0, 1)
			}
			if col == 0 {
				return accountStyle.Padding(0, 1)
			}
			return whoamiValStyle.Padding(0, 1)
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
		name := strings.TrimSuffix(e.Name(), ".json")
		if err := validateAccountName(name); err != nil {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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

func oauthAccountIdentity(oauth any) (string, bool) {
	m, ok := oauth.(map[string]any)
	if !ok || m == nil {
		return "", false
	}
	if uuid, ok := stringField(m, "accountUuid"); ok {
		return "uuid:" + uuid, true
	}
	if email, ok := stringField(m, "emailAddress"); ok {
		return "email:" + strings.ToLower(email), true
	}
	return "", false
}

func sameOAuthAccount(a, b any) bool {
	idA, okA := oauthAccountIdentity(a)
	idB, okB := oauthAccountIdentity(b)
	if okA && okB {
		return idA == idB
	}
	return jsonEqual(a, b)
}

func isActiveSavedAccount(name string, active any) bool {
	if active == nil {
		return false
	}
	saved, ok := savedOAuthAccount(name)
	if !ok {
		return false
	}
	return sameOAuthAccount(active, saved)
}
