package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

type claudeSession struct {
	PID       int    `json:"pid"`
	ProcStart string `json:"procStart"`
}

func claudeSessionsDir() string {
	return filepath.Join(claudeDir(), "sessions")
}

func activeClaudeSessionCount() (int, error) {
	entries, err := os.ReadDir(claudeSessionsDir())
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("reading Claude sessions: %w", err)
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(claudeSessionsDir(), entry.Name()))
		if err != nil {
			return 0, fmt.Errorf("reading Claude session %s: %w", entry.Name(), err)
		}
		var session claudeSession
		if err := json.Unmarshal(raw, &session); err != nil {
			continue
		}
		if claudeSessionProcessAlive(session) {
			count++
		}
	}
	return count, nil
}

func claudeSessionProcessAlive(session claudeSession) bool {
	if session.PID <= 0 {
		return false
	}

	if runtime.GOOS == "linux" {
		raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(session.PID), "stat"))
		if err != nil {
			return false
		}
		stat := string(raw)
		commandEnd := strings.LastIndex(stat, ")")
		if commandEnd < 0 {
			return false
		}
		fields := strings.Fields(stat[commandEnd+1:])
		const startTimeIndex = 19
		return len(fields) > startTimeIndex &&
			session.ProcStart != "" &&
			fields[startTimeIndex] == session.ProcStart
	}

	return true
}
