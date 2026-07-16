package main

import (
	"embed"
	"fmt"
	"os"
)

//go:embed completions/_cc-switch completions/cc-switch.bash completions/cc-switch.fish
var completionFiles embed.FS

var completionPaths = map[string]string{
	"bash": "completions/cc-switch.bash",
	"fish": "completions/cc-switch.fish",
	"zsh":  "completions/_cc-switch",
}

func cmdCompletion(shell string) {
	path, ok := completionPaths[shell]
	if !ok {
		fmt.Fprintln(os.Stderr, "Usage: cc-switch completion {bash|fish|zsh}")
		os.Exit(1)
	}

	data, err := completionFiles.ReadFile(path)
	if err != nil {
		fatalf("could not load %s completion: %v", shell, err)
	}
	if _, err := os.Stdout.Write(data); err != nil {
		fatalf("could not write %s completion: %v", shell, err)
	}
}
