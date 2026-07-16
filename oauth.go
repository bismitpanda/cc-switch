package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	oauthClientID     = "9d1c250a-e61b-44d9-88ed-5944d1962f5e"
	oauthTokenURL     = "https://platform.claude.com/v1/oauth/token"
	oauthScopes       = "user:profile user:inference user:sessions:claude_code user:mcp_servers user:file_upload"
	oauthExpiryBuffer = 5 * time.Minute
)

var accountRefreshMu sync.Map // name -> *sync.Mutex

func accountMutex(name string) *sync.Mutex {
	v, _ := accountRefreshMu.LoadOrStore(name, &sync.Mutex{})
	return v.(*sync.Mutex)
}

type oauthRefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
}

func accountClaudeAiOauth(name string) (map[string]any, map[string]any, error) {
	if err := validateAccountName(name); err != nil {
		return nil, nil, err
	}
	snap, err := readJSONObject(accountSnapPath(name))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("no saved account called '%s'", name)
		}
		return nil, nil, err
	}
	oauthVal, ok := snap["claudeAiOauth"]
	if !ok || oauthVal == nil {
		return nil, nil, fmt.Errorf("account '%s' has no claudeAiOauth credentials", name)
	}
	oauth, ok := oauthVal.(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("account '%s' has invalid claudeAiOauth credentials", name)
	}
	return snap, oauth, nil
}

func stringField(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok || v == nil {
		return "", false
	}
	s, ok := v.(string)
	return s, ok && s != ""
}

func expiresAtMs(m map[string]any) (int64, bool) {
	v, ok := m["expiresAt"]
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

func oauthTokenExpired(oauth map[string]any) bool {
	exp, ok := expiresAtMs(oauth)
	if !ok {
		return false
	}
	return time.Now().Add(oauthExpiryBuffer).UnixMilli() >= exp
}

func refreshOAuthCredentials(refreshToken string) (oauthRefreshResponse, error) {
	body := map[string]string{
		"grant_type":    "refresh_token",
		"refresh_token": refreshToken,
		"client_id":     oauthClientID,
		"scope":         oauthScopes,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return oauthRefreshResponse{}, err
	}

	req, err := http.NewRequest(http.MethodPost, oauthTokenURL, bytes.NewReader(payload))
	if err != nil {
		return oauthRefreshResponse{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return oauthRefreshResponse{}, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return oauthRefreshResponse{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return oauthRefreshResponse{}, fmt.Errorf("token refresh failed (%s): %s", resp.Status, strings.TrimSpace(string(raw)))
	}

	var parsed oauthRefreshResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return oauthRefreshResponse{}, fmt.Errorf("parsing refresh response: %w", err)
	}
	if parsed.AccessToken == "" {
		return oauthRefreshResponse{}, fmt.Errorf("refresh response missing access_token")
	}
	if parsed.ExpiresIn <= 0 {
		parsed.ExpiresIn = 3600
	}
	if parsed.RefreshToken == "" {
		parsed.RefreshToken = refreshToken
	}
	return parsed, nil
}

func applyRefreshedTokens(oauth map[string]any, refreshed oauthRefreshResponse) {
	oauth["accessToken"] = refreshed.AccessToken
	oauth["refreshToken"] = refreshed.RefreshToken
	oauth["expiresAt"] = time.Now().UnixMilli() + refreshed.ExpiresIn*1000
	if refreshed.Scope != "" {
		scopes := strings.Fields(refreshed.Scope)
		if len(scopes) > 0 {
			oauth["scopes"] = scopes
		}
	}
}

func persistAccountOAuth(name string, snap map[string]any, oauth map[string]any) error {
	snap["claudeAiOauth"] = oauth
	if err := writeJSONObject(accountSnapPath(name), snap, 0600); err != nil {
		return fmt.Errorf("could not update snapshot for %s: %w", name, err)
	}
	active, _ := activeOAuthAccount()
	if isActiveSavedAccount(name, active) {
		return writeLiveClaudeAiOauth(oauth)
	}
	return nil
}

func writeLiveClaudeAiOauth(oauth map[string]any) error {
	cf := credFile()
	cred, err := readJSONObject(cf)
	if err != nil {
		if os.IsNotExist(err) {
			cred = map[string]any{}
		} else {
			return err
		}
	}
	cred["claudeAiOauth"] = oauth
	return writeJSONObject(cf, cred, 0600)
}

func refreshAccountTokens(name string) (string, error) {
	mu := accountMutex(name)
	mu.Lock()
	defer mu.Unlock()

	snap, oauth, err := accountClaudeAiOauth(name)
	if err != nil {
		return "", err
	}
	refreshToken, ok := stringField(oauth, "refreshToken")
	if !ok {
		return "", fmt.Errorf("account '%s' has no refresh token — re-login and save", name)
	}
	refreshed, err := refreshOAuthCredentials(refreshToken)
	if err != nil {
		return "", fmt.Errorf("account '%s' token refresh failed: %w", name, err)
	}
	applyRefreshedTokens(oauth, refreshed)
	if err := persistAccountOAuth(name, snap, oauth); err != nil {
		return "", err
	}
	return refreshed.AccessToken, nil
}

func ensureAccountAccessToken(name string) (string, error) {
	mu := accountMutex(name)
	mu.Lock()
	defer mu.Unlock()

	snap, oauth, err := accountClaudeAiOauth(name)
	if err != nil {
		return "", err
	}
	if oauthTokenExpired(oauth) {
		refreshToken, ok := stringField(oauth, "refreshToken")
		if !ok {
			return "", fmt.Errorf("account '%s' token expired and has no refresh token — re-login and save", name)
		}
		refreshed, err := refreshOAuthCredentials(refreshToken)
		if err != nil {
			return "", fmt.Errorf("account '%s' token refresh failed: %w", name, err)
		}
		applyRefreshedTokens(oauth, refreshed)
		if err := persistAccountOAuth(name, snap, oauth); err != nil {
			return "", err
		}
		return refreshed.AccessToken, nil
	}
	token, ok := stringField(oauth, "accessToken")
	if !ok {
		return "", fmt.Errorf("account '%s' has no access token", name)
	}
	return token, nil
}
