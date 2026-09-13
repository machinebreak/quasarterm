// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	claudeAuthorizeURL   = "https://claude.com/cai/oauth/authorize"
	claudeManualRedirect = "https://platform.claude.com/oauth/code/callback"
	codexDeviceUserCodeURL = "https://auth.openai.com/api/accounts/deviceauth/usercode"
	codexDeviceTokenURL    = "https://auth.openai.com/api/accounts/deviceauth/token"
	codexDeviceVerifyURL   = "https://auth.openai.com/codex/device"
	codexDeviceRedirectURI = "https://auth.openai.com/deviceauth/callback"
)

var claudeOAuthScopes = []string{
	"org:create_api_key",
	"user:profile",
	"user:inference",
	"user:sessions:claude_code",
	"user:mcp_servers",
	"user:file_upload",
}

func randomURLToken(length int) string {
	bytesNeeded := (length*3 + 3) / 4
	raw := make([]byte, bytesNeeded)
	if _, err := rand.Read(raw); err != nil {
		return hex.EncodeToString(raw)
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	if len(encoded) > length {
		encoded = encoded[:length]
	}
	return encoded
}

func startClaudeOAuthLogin() (*LoginStart, error) {
	sessionID := loginSessionID()
	state := randomURLToken(32)
	verifier := randomURLToken(32)
	challengeDigest := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(challengeDigest[:])
	authorizeURL, err := url.Parse(claudeAuthorizeURL)
	if err != nil {
		return nil, err
	}
	params := authorizeURL.Query()
	params.Set("code", "true")
	params.Set("client_id", claudeOAuthClientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", claudeManualRedirect)
	params.Set("scope", strings.Join(claudeOAuthScopes, " "))
	params.Set("code_challenge", challenge)
	params.Set("code_challenge_method", "S256")
	params.Set("state", state)
	authorizeURL.RawQuery = params.Encode()
	loginSessions.Lock()
	loginSessions.items[sessionID] = &loginSession{
		provider:  "claude",
		state:     state,
		verifier:  verifier,
		authURL:   authorizeURL.String(),
		expiresAt: time.Now().Add(10 * time.Minute),
	}
	loginSessions.Unlock()
	return &LoginStart{
		SessionID:       sessionID,
		Provider:        "claude",
		VerificationURI: authorizeURL.String(),
		ExpiresIn:       600,
	}, nil
}

func parseClaudeCodeInput(input string) (string, string, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", "", fmt.Errorf("authorization code cannot be empty")
	}
	if parsed, err := url.Parse(trimmed); err == nil && parsed.Host != "" {
		if strings.EqualFold(parsed.Host, "claude.com") || strings.EqualFold(parsed.Host, "www.claude.com") {
			if strings.EqualFold(parsed.Path, "/cai/oauth/authorize") {
				return "", "", fmt.Errorf("pasted URL is the authorization page, not the final code")
			}
		}
		query := parsed.Query()
		code := query.Get("code")
		if code != "" {
			return code, query.Get("state"), nil
		}
	}
	code := trimmed
	state := ""
	if before, after, found := strings.Cut(code, "#"); found {
		code = before
		state = strings.TrimSpace(after)
	}
	if before, _, found := strings.Cut(code, "&"); found {
		code = before
	}
	return strings.TrimSpace(code), state, nil
}

func claudeSubscriptionType(profile map[string]any) string {
	organization, _ := profile["organization"].(map[string]any)
	orgType := jsonString(organization["organization_type"])
	switch orgType {
	case "claude_max":
		return "Max"
	case "claude_pro":
		return "Pro"
	case "claude_enterprise":
		return "Enterprise"
	case "claude_team":
		return "Team"
	}
	return ""
}

func SubmitBrowserLoginCode(ctx context.Context, sessionID string, input string) (*Account, error) {
	loginSessions.Lock()
	session, found := loginSessions.items[sessionID]
	loginSessions.Unlock()
	if !found {
		return nil, fmt.Errorf("login session not found")
	}
	if time.Now().After(session.expiresAt) {
		loginSessions.Lock()
		delete(loginSessions.items, sessionID)
		loginSessions.Unlock()
		return nil, fmt.Errorf("login session expired")
	}
	code, state, err := parseClaudeCodeInput(input)
	if err != nil {
		return nil, err
	}
	if state != "" && session.state != "" && state != session.state {
		return nil, fmt.Errorf("OAuth state mismatch, please start over")
	}
	payload, _ := json.Marshal(map[string]string{
		"grant_type":    "authorization_code",
		"client_id":     claudeOAuthClientID,
		"code":          code,
		"redirect_uri":  claudeManualRedirect,
		"code_verifier": session.verifier,
		"state":         session.state,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, claudeTokenEndpoint, strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", "quasar")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Claude token exchange failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Claude token exchange failed (%d): %s", resp.StatusCode, summarizeBody(body))
	}
	var tokenResponse struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
		Scope        string `json:"scope"`
	}
	if err := json.Unmarshal(body, &tokenResponse); err != nil {
		return nil, err
	}
	if tokenResponse.AccessToken == "" {
		return nil, fmt.Errorf("Claude token exchange returned an empty access token")
	}
	profile := map[string]any{}
	if profileReq, reqErr := newRequest(ctx, http.MethodGet, claudeProfileURL); reqErr == nil {
		profileReq.Header.Set("Authorization", "Bearer "+tokenResponse.AccessToken)
		profileReq.Header.Set("anthropic-beta", claudeBetaHeader)
		profileReq.Header.Set("anthropic-version", "2023-06-01")
		if profileResp, respErr := httpClient.Do(profileReq); respErr == nil {
			defer profileResp.Body.Close()
			if profileResp.StatusCode == http.StatusOK {
				profileBody, _ := io.ReadAll(io.LimitReader(profileResp.Body, 1<<20))
				json.Unmarshal(profileBody, &profile)
			}
		}
	}
	scopes := claudeOAuthScopes
	if strings.TrimSpace(tokenResponse.Scope) != "" {
		scopes = strings.Fields(tokenResponse.Scope)
	}
	oauth := map[string]any{
		"accessToken":  tokenResponse.AccessToken,
		"refreshToken": tokenResponse.RefreshToken,
		"expiresAt":    nowMillis() + tokenResponse.ExpiresIn*1000,
		"scopes":       scopes,
	}
	if subscription := claudeSubscriptionType(profile); subscription != "" {
		oauth["subscriptionType"] = subscription
	}
	credentials, _ := json.MarshalIndent(map[string]any{"claudeAiOauth": oauth}, "", "  ")
	accountInfo, _ := profile["account"].(map[string]any)
	email := firstNonEmpty(jsonString(accountInfo["email"]), jsonString(profile["email"]))
	label := firstNonEmpty(email, jsonString(accountInfo["display_name"]), "Claude account")
	remoteKey := hashKey(tokenResponse.RefreshToken)[:24]
	account := &Account{
		ID:        makeAccountID("claude", remoteKey),
		Provider:  "claude",
		Email:     email,
		PlanType:  firstNonEmpty(claudeSubscriptionType(profile), ""),
		RemoteID:  remoteKey,
		CreatedAt: nowMillis(),
		Label:     label,
	}
	if err := GetManager().AddExternalAccount("claude", account, credentials); err != nil {
		return nil, err
	}
	loginSessions.Lock()
	delete(loginSessions.items, sessionID)
	loginSessions.Unlock()
	return account, nil
}

const grokOIDCScope = "openid profile email offline_access grok-cli:access api:access conversations:read conversations:write"

type LoginStart struct {
	SessionID               string `json:"sessionid"`
	Provider                string `json:"provider"`
	UserCode                string `json:"usercode,omitempty"`
	VerificationURI         string `json:"verificationuri,omitempty"`
	VerificationURIComplete string `json:"verificationuricomplete,omitempty"`
	ExpiresIn               int64  `json:"expiresin,omitempty"`
	Interval                int64  `json:"interval,omitempty"`
}

type loginSession struct {
	provider   string
	deviceCode string
	tokenURL   string
	interval   int64
	state      string
	verifier   string
	authURL    string
	expiresAt  time.Time
}

var loginSessions = struct {
	sync.Mutex
	items map[string]*loginSession
}{items: map[string]*loginSession{}}

func loginSessionID() string {
	return fmt.Sprintf("login_%d", time.Now().UnixNano())
}

func fetchGrokDiscovery(ctx context.Context) (string, string, error) {
	fallbackAuth := "https://auth.x.ai/oauth2/device/authorize"
	fallbackToken := grokTokenEndpoint
	req, err := newRequest(ctx, http.MethodGet, "https://auth.x.ai/.well-known/openid-configuration")
	if err != nil {
		return fallbackAuth, fallbackToken, nil
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fallbackAuth, fallbackToken, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fallbackAuth, fallbackToken, nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var discovery struct {
		DeviceAuthorizationEndpoint string `json:"device_authorization_endpoint"`
		TokenEndpoint               string `json:"token_endpoint"`
	}
	if err := json.Unmarshal(body, &discovery); err != nil {
		return fallbackAuth, fallbackToken, nil
	}
	authEndpoint := fallbackAuth
	if strings.HasPrefix(discovery.DeviceAuthorizationEndpoint, "https://auth.x.ai") ||
		strings.HasPrefix(discovery.DeviceAuthorizationEndpoint, "https://accounts.x.ai") {
		authEndpoint = discovery.DeviceAuthorizationEndpoint
	}
	tokenEndpoint := fallbackToken
	if strings.HasPrefix(discovery.TokenEndpoint, "https://auth.x.ai") ||
		strings.HasPrefix(discovery.TokenEndpoint, "https://accounts.x.ai") {
		tokenEndpoint = discovery.TokenEndpoint
	}
	return authEndpoint, tokenEndpoint, nil
}

func startGrokDeviceLogin(ctx context.Context) (*LoginStart, error) {
	authEndpoint, tokenEndpoint, err := fetchGrokDiscovery(ctx)
	if err != nil {
		return nil, err
	}
	form := url.Values{}
	form.Set("client_id", grokOAuthClientID)
	form.Set("scope", grokOIDCScope)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, authEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Grok device flow failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Grok device flow returned %d: %s", resp.StatusCode, summarizeBody(body))
	}
	var payload struct {
		DeviceCode              string `json:"device_code"`
		UserCode                string `json:"user_code"`
		VerificationURI         string `json:"verification_uri"`
		VerificationURIComplete string `json:"verification_uri_complete"`
		ExpiresIn               int64  `json:"expires_in"`
		Interval                int64  `json:"interval"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("error parsing Grok device flow response: %w", err)
	}
	if payload.DeviceCode == "" || payload.UserCode == "" {
		return nil, fmt.Errorf("Grok device flow response is incomplete")
	}
	if payload.Interval <= 0 {
		payload.Interval = 5
	}
	sessionID := loginSessionID()
	loginSessions.Lock()
	loginSessions.items[sessionID] = &loginSession{
		provider:   "grok",
		deviceCode: payload.DeviceCode,
		tokenURL:   tokenEndpoint,
		interval:   payload.Interval,
		expiresAt:  time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second),
	}
	loginSessions.Unlock()
	return &LoginStart{
		SessionID:               sessionID,
		Provider:                "grok",
		UserCode:                payload.UserCode,
		VerificationURI:         payload.VerificationURI,
		VerificationURIComplete: payload.VerificationURIComplete,
		ExpiresIn:               payload.ExpiresIn,
		Interval:                payload.Interval,
	}, nil
}

func StartBrowserLogin(ctx context.Context, providerID string) (*LoginStart, error) {
	switch providerID {
	case "grok":
		return startGrokDeviceLogin(ctx)
	case "claude":
		return startClaudeOAuthLogin()
	case "codex":
		return startCodexDeviceLogin(ctx)
	default:
		return nil, fmt.Errorf("browser login is not supported for %s yet", providerID)
	}
}

func startCodexDeviceLogin(ctx context.Context) (*LoginStart, error) {
	payload, _ := json.Marshal(map[string]string{"client_id": codexOAuthClientID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, codexDeviceUserCodeURL, strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Codex device authorization failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Codex device authorization returned %d: %s", resp.StatusCode, summarizeBody(body))
	}
	var payloadResponse struct {
		DeviceAuthID string `json:"device_auth_id"`
		UserCode     string `json:"user_code"`
		Usercode     string `json:"usercode"`
		Interval     int64  `json:"interval"`
	}
	if err := json.Unmarshal(body, &payloadResponse); err != nil {
		return nil, err
	}
	userCode := firstNonEmpty(payloadResponse.UserCode, payloadResponse.Usercode)
	if payloadResponse.DeviceAuthID == "" || userCode == "" {
		return nil, fmt.Errorf("Codex device authorization response is incomplete")
	}
	if payloadResponse.Interval <= 0 {
		payloadResponse.Interval = 5
	}
	sessionID := loginSessionID()
	loginSessions.Lock()
	loginSessions.items[sessionID] = &loginSession{
		provider:   "codex",
		deviceCode: payloadResponse.DeviceAuthID,
		tokenURL:   codexDeviceTokenURL,
		interval:   payloadResponse.Interval,
		state:      userCode,
		expiresAt:  time.Now().Add(15 * time.Minute),
	}
	loginSessions.Unlock()
	return &LoginStart{
		SessionID:               sessionID,
		Provider:                "codex",
		UserCode:                userCode,
		VerificationURI:         codexDeviceVerifyURL,
		VerificationURIComplete: codexDeviceVerifyURL,
		ExpiresIn:               900,
		Interval:                payloadResponse.Interval,
	}, nil
}

func exchangeCodexTokens(ctx context.Context, code string, verifier string) (string, string, string, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", codexDeviceRedirectURI)
	form.Set("client_id", codexOAuthClientID)
	form.Set("code_verifier", verifier)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, codexTokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", "", fmt.Errorf("Codex token exchange failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("Codex token exchange failed (%d): %s", resp.StatusCode, summarizeBody(body))
	}
	var tokenResponse struct {
		IDToken      string `json:"id_token"`
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.Unmarshal(body, &tokenResponse); err != nil {
		return "", "", "", err
	}
	if tokenResponse.AccessToken == "" || tokenResponse.IDToken == "" {
		return "", "", "", fmt.Errorf("Codex token exchange response is missing tokens")
	}
	return tokenResponse.IDToken, tokenResponse.AccessToken, tokenResponse.RefreshToken, nil
}

func completeCodexLogin(ctx context.Context, session *loginSession) (*Account, error) {
	payload, _ := json.Marshal(map[string]string{
		"device_auth_id": session.deviceCode,
		"user_code":      session.state,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, session.tokenURL, strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusForbidden || resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Codex device authorization failed (%d): %s", resp.StatusCode, summarizeBody(body))
	}
	var deviceResponse struct {
		AuthorizationCode string `json:"authorization_code"`
		CodeVerifier      string `json:"code_verifier"`
		CodeChallenge     string `json:"code_challenge"`
	}
	if err := json.Unmarshal(body, &deviceResponse); err != nil {
		return nil, err
	}
	if deviceResponse.AuthorizationCode == "" || deviceResponse.CodeVerifier == "" {
		return nil, fmt.Errorf("Codex device response is missing the authorization code")
	}
	idToken, accessToken, refreshToken, err := exchangeCodexTokens(ctx, deviceResponse.AuthorizationCode, deviceResponse.CodeVerifier)
	if err != nil {
		return nil, err
	}
	tokens := &codexAuthTokens{IDToken: idToken, AccessToken: accessToken, RefreshToken: refreshToken}
	email, accountID, planType := codexIdentityFromTokens(tokens)
	authFile := map[string]any{
		"auth_mode":     "chatgpt",
		"OPENAI_API_KEY": nil,
		"tokens": map[string]any{
			"id_token":      idToken,
			"access_token":  accessToken,
			"refresh_token": refreshToken,
		},
		"last_refresh": time.Now().UTC().Format(time.RFC3339),
	}
	credentials, _ := json.MarshalIndent(authFile, "", "  ")
	remoteKey := accountID
	if remoteKey == "" {
		remoteKey = hashKey(accessToken)[:24]
	}
	label := firstNonEmpty(email, "Codex account")
	account := &Account{
		ID:        makeAccountID("codex", remoteKey),
		Provider:  "codex",
		Email:     email,
		PlanType:  planType,
		RemoteID:  remoteKey,
		CreatedAt: nowMillis(),
		Label:     label,
	}
	if err := GetManager().AddExternalAccount("codex", account, credentials); err != nil {
		return nil, err
	}
	return account, nil
}

func PollBrowserLogin(ctx context.Context, sessionID string) (*Account, error) {
	loginSessions.Lock()
	session, found := loginSessions.items[sessionID]
	loginSessions.Unlock()
	if !found {
		return nil, fmt.Errorf("login session not found")
	}
	if time.Now().After(session.expiresAt) {
		loginSessions.Lock()
		delete(loginSessions.items, sessionID)
		loginSessions.Unlock()
		return nil, fmt.Errorf("login session expired")
	}
	switch session.provider {
	case "grok":
		return pollGrokSession(ctx, sessionID, session)
	case "codex":
		account, err := completeCodexLogin(ctx, session)
		if err != nil || account == nil {
			return account, err
		}
		loginSessions.Lock()
		delete(loginSessions.items, sessionID)
		loginSessions.Unlock()
		return account, nil
	default:
		return nil, fmt.Errorf("login polling is not supported for %s", session.provider)
	}
}

func pollGrokSession(ctx context.Context, sessionID string, session *loginSession) (*Account, error) {
	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")
	form.Set("device_code", session.deviceCode)
	form.Set("client_id", grokOAuthClientID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, session.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		var errorPayload struct {
			Error string `json:"error"`
		}
		json.Unmarshal(body, &errorPayload)
		switch errorPayload.Error {
		case "authorization_pending", "slow_down":
			return nil, nil
		default:
			return nil, fmt.Errorf("Grok authorization failed: %s", firstNonEmpty(errorPayload.Error, summarizeBody(body)))
		}
	}
	var tokenPayload struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenPayload); err != nil {
		return nil, err
	}
	if tokenPayload.AccessToken == "" {
		return nil, fmt.Errorf("Grok authorization returned an empty access token")
	}
	entry := map[string]any{
		"access_token": tokenPayload.AccessToken,
		"token_type":   "Bearer",
	}
	if tokenPayload.RefreshToken != "" {
		entry["refresh_token"] = tokenPayload.RefreshToken
	}
	if tokenPayload.ExpiresIn > 0 {
		entry["expires_at"] = time.Now().Unix() + tokenPayload.ExpiresIn
	}
	entry["oidc_client_id"] = grokOAuthClientID
	creds := grokCredentials{RegistryKey: grokAuthRegistryKey, Entry: entry}
	credentials, _ := json.MarshalIndent(creds, "", "  ")
	email := ""
	if claims := decodeJWTPayload(tokenPayload.AccessToken); claims != nil {
		email = firstNonEmpty(jsonString(claims["email"]), jsonString(claims["preferred_username"]))
	}
	label := firstNonEmpty(email, "Grok account")
	account := &Account{
		ID:        makeAccountID("grok", hashKey(tokenPayload.AccessToken)[:24]),
		Provider:  "grok",
		Email:     email,
		RemoteID:  hashKey(tokenPayload.AccessToken)[:24],
		CreatedAt: nowMillis(),
		Label:     label,
	}
	if err := GetManager().AddExternalAccount("grok", account, credentials); err != nil {
		return nil, err
	}
	loginSessions.Lock()
	delete(loginSessions.items, sessionID)
	loginSessions.Unlock()
	return account, nil
}
