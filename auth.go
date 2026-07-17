package main

import (
	"encoding/json"
	"fmt"
	"time"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
)

type tokenState int

const (
	tokenStateMissing tokenState = iota
	tokenStateValid
	tokenStateExpired
)

type tokenStatus struct {
	text  string
	state tokenState
}

type authStatusRow struct {
	account string
	active  bool
	access  tokenStatus
	refresh tokenStatus
}

func cmdStatus() {
	initStyles()

	active, _ := activeOAuthAccount()
	names := listAccountNames()
	rows := make([]authStatusRow, 0, len(names)+1)
	hasSavedActive := false
	now := time.Now()

	for _, name := range names {
		isActive := isActiveSavedAccount(name, active)
		hasSavedActive = hasSavedActive || isActive

		var (
			oauth map[string]any
			err   error
		)
		if isActive {
			oauth, err = liveClaudeAiOauth()
		} else {
			_, oauth, err = accountClaudeAiOauth(name)
		}
		rows = append(rows, makeAuthStatusRow(name, isActive, oauth, err, now))
	}

	if active != nil && !hasSavedActive {
		oauth, err := liveClaudeAiOauth()
		rows = append(rows, makeAuthStatusRow("(current, unsaved)", true, oauth, err, now))
	}

	if len(rows) == 0 {
		printMuted("(no saved or active credentials)")
		return
	}

	tableRows := make([][]string, len(rows))
	for i, row := range rows {
		account := row.account
		if row.active {
			account = account + " ●"
		}
		tableRows[i] = []string{account, row.access.text, row.refresh.text}
	}

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(mutedStyle).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return labelStyle.Bold(true).Padding(0, 1)
			}
			if col == 0 {
				if rows[row].active {
					return accountStyle.Bold(true).Foreground(lipgloss.Color("42")).Padding(0, 1)
				}
				return accountStyle.Padding(0, 1)
			}
			if col == 1 || col == 2 {
				state := rows[row].access.state
				if col == 2 {
					state = rows[row].refresh.state
				}
				switch state {
				case tokenStateValid:
					return successStyle.Padding(0, 1)
				case tokenStateExpired, tokenStateMissing:
					return errorStyle.Padding(0, 1)
				}
			}
			return whoamiValStyle.Padding(0, 1)
		}).
		Headers("Account", "Access token", "Refresh token").
		Rows(tableRows...)
	lipgloss.Println(t)
}

func makeAuthStatusRow(name string, active bool, oauth map[string]any, loadErr error, now time.Time) authStatusRow {
	row := authStatusRow{
		account: name,
		active:  active,
		access:  tokenStatus{text: "Missing", state: tokenStateMissing},
		refresh: tokenStatus{text: "Missing", state: tokenStateMissing},
	}
	if loadErr != nil {
		return row
	}

	row.access = makeTokenStatus(oauth, "accessToken", "expiresAt", now)
	row.refresh = makeTokenStatus(oauth, "refreshToken", "refreshTokenExpiresAt", now)
	return row
}

func makeTokenStatus(oauth map[string]any, tokenKey, expiryKey string, now time.Time) tokenStatus {
	if _, ok := stringField(oauth, tokenKey); !ok {
		return tokenStatus{text: "Missing", state: tokenStateMissing}
	}
	expiryMs, ok := millisecondField(oauth, expiryKey)
	if !ok {
		return tokenStatus{text: "Valid (expiry unknown)", state: tokenStateValid}
	}

	expiry := time.UnixMilli(expiryMs)
	when := formatTokenExpiry(expiry, now)
	if expiryMs <= now.UnixMilli() {
		return tokenStatus{text: fmt.Sprintf("Expired (%s)", when), state: tokenStateExpired}
	}
	return tokenStatus{text: fmt.Sprintf("Valid (%s)", when), state: tokenStateValid}
}

func millisecondField(m map[string]any, key string) (int64, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int64(n), true
	case int64:
		return n, true
	case json.Number:
		i, err := n.Int64()
		return i, err == nil
	default:
		return 0, false
	}
}

func formatTokenExpiry(expiry, now time.Time) string {
	localExpiry := expiry.Local()
	localNow := now.Local()
	when := localExpiry.Format("Jan 2, 3:04 PM")
	if localExpiry.Year() == localNow.Year() && localExpiry.YearDay() == localNow.YearDay() {
		when = localExpiry.Format("3:04 PM")
	}
	return when
}
