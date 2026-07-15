package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func readJSONObject(path string) (map[string]interface{}, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]interface{}{}, nil
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if m == nil {
		m = map[string]interface{}{}
	}
	return m, nil
}

func writeJSONObject(path string, v interface{}, perm os.FileMode) error {
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

func orDefault(v interface{}, ok bool, def interface{}) interface{} {
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
		fatalf("No credentials file at %s yet — run 'claude' then '/login' first.", cf)
	}

	global, err := readJSONObject(globalFile())
	if err != nil {
		fatalf("%v", err)
	}
	oauthVal, ok := global["oauthAccount"]
	oauth := orDefault(oauthVal, ok, map[string]interface{}{})

	cred, err := readJSONObject(cf)
	if err != nil {
		fatalf("%v", err)
	}
	credVal, ok := cred["claudeAiOauth"]
	claudeAiOauth := orDefault(credVal, ok, map[string]interface{}{})

	snap := map[string]interface{}{
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
		credFileObj = map[string]interface{}{}
	}
	credFileObj["claudeAiOauth"] = cred
	if err := writeJSONObject(cf, credFileObj, 0600); err != nil {
		fatalf("could not write %s: %v", cf, err)
	}

	fmt.Printf("Switched active account to '%s'. (If Claude Code is already running, exit and relaunch it.)\n", name)
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
	fmt.Print(`cc-switch — switch the active Claude Code account without changing config directories.

Only two keys are ever read or written:
  ~/.claude.json              oauthAccount
  ~/.claude/.credentials.json claudeAiOauth

Everything else in those files (projects, settings, MCP OAuth, etc.) is left untouched.

Usage:
  cc-switch save <name>   snapshot the currently logged-in account
  cc-switch use <name>    switch to a saved account
  cc-switch list          list saved account names
  cc-switch whoami        show the active oauthAccount
  cc-switch help          show this help

First-time setup for multiple accounts:
  claude                  # launch, then /login with account A
  cc-switch save personal
  claude /logout
  claude                  # /login with account B
  cc-switch save work
  cc-switch use personal  # switch without another browser login
  cc-switch use work
`)
}

func usage() {
	fmt.Fprintln(os.Stderr, "Usage: cc-switch {save <name>|use <name>|list|whoami|help}")
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
	case "help", "-h", "--help":
		cmdHelp()
	default:
		usage()
	}
}
