// Copyright 2026, Quasar
// SPDX-License-Identifier: Apache-2.0

package accounts

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/wavetermdev/waveterm/pkg/wavebase"
)

const (
	traeDefaultProviderID   = "icube.cloudide"
	traeAuthKeyPrefix       = "iCubeAuthInfo://"
	traeServerKeyPrefix     = "iCubeServerData://"
	traeEntitlementKeyPrefix = "iCubeEntitlementInfo://"
	traeDeviceKeyPrefix     = "iCubeAuthInfo://icube-dc:"
	traeUsertagKey          = "iCubeAuthInfo://usertag"
)

type traeVariant struct {
	id          string
	name        string
	description string
	icon        string
	order       float64
	dirName     string
}

var traeVariants = map[string]traeVariant{
	"trae":         {id: "trae", name: "Trae", description: "Trae IDE", icon: "trae", order: 100, dirName: "Trae"},
	"trae-solo":    {id: "trae-solo", name: "TRAE SOLO", description: "TRAE SOLO", icon: "trae-solo", order: 105, dirName: "TRAE SOLO"},
	"trae-cn":      {id: "trae-cn", name: "Trae CN", description: "Trae China", icon: "trae-cn", order: 110, dirName: "Trae CN"},
	"trae-solo-cn": {id: "trae-solo-cn", name: "TRAE SOLO CN", description: "TRAE SOLO China", icon: "trae-solo-cn", order: 115, dirName: "TRAE SOLO CN"},
}

type traeProvider struct {
	variant traeVariant
}

func newTraeProvider(variantKey string) *traeProvider {
	return &traeProvider{variant: traeVariants[variantKey]}
}

func (p *traeProvider) Info() ProviderInfo {
	v := p.variant
	return ProviderInfo{ID: v.id, Name: v.name, Description: v.description, CLICommand: v.id,
		Icon: v.icon, Order: v.order, Available: true}
}

func traeStoragePath(variant traeVariant) (string, error) {
	appData := os.Getenv("APPDATA")
	candidates := []string{}
	if appData != "" {
		candidates = append(candidates, filepath.Join(appData, variant.dirName, "User", "globalStorage", "storage.json"))
	}
	candidates = append(candidates, filepath.Join(wavebase.GetHomeDir(), ".config", variant.dirName, "User", "globalStorage", "storage.json"))
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("%s storage.json not found (log in to %s first)", variant.name, variant.name)
}

func traeNestedString(obj map[string]any, paths ...[]string) string {
	for _, path := range paths {
		var current any = obj
		for _, key := range path {
			m, ok := current.(map[string]any)
			if !ok {
				current = nil
				break
			}
			current = m[key]
		}
		if value := jsonString(current); value != "" {
			return value
		}
	}
	return ""
}

type traeStoredCredentials struct {
	ProviderID string         `json:"providerId"`
	AuthKey    string         `json:"authKey"`
	AuthRaw    map[string]any `json:"authRaw"`
}

func parseTraeCredentials(credentials []byte) (*traeStoredCredentials, error) {
	var creds traeStoredCredentials
	if err := json.Unmarshal(credentials, &creds); err != nil {
		return nil, fmt.Errorf("invalid Trae credentials: %w", err)
	}
	if creds.AuthRaw == nil || creds.AuthKey == "" {
		return nil, fmt.Errorf("Trae credentials are incomplete")
	}
	return &creds, nil
}

func traeFindAuthKey(root map[string]any) string {
	for key := range root {
		if strings.HasPrefix(key, traeAuthKeyPrefix) && key != traeUsertagKey && !strings.HasPrefix(key, traeDeviceKeyPrefix) {
			return key
		}
	}
	return traeAuthKeyPrefix + traeDefaultProviderID
}

func (p *traeProvider) readStorage() (map[string]any, string, error) {
	storePath, err := traeStoragePath(p.variant)
	if err != nil {
		return nil, "", err
	}
	raw, err := os.ReadFile(storePath)
	if err != nil {
		return nil, "", err
	}
	root := map[string]any{}
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, "", fmt.Errorf("invalid %s storage.json: %w", p.variant.name, err)
	}
	return root, storePath, nil
}

func traeAuthTokenFromRaw(authRaw map[string]any, serverRaw map[string]any) string {
	token := traeNestedString(authRaw,
		[]string{"accessToken"}, []string{"access_token"}, []string{"token"},
		[]string{"data", "accessToken"}, []string{"auth", "accessToken"}, []string{"auth", "token"})
	if token == "" && serverRaw != nil {
		token = traeNestedString(serverRaw,
			[]string{"accessToken"}, []string{"access_token"}, []string{"token"}, []string{"data", "accessToken"})
	}
	return token
}

func (p *traeProvider) CurrentCredentialsPath() (string, error) {
	return traeStoragePath(p.variant)
}

func (p *traeProvider) ImportCurrent() (*Account, []byte, error) {
	root, _, err := p.readStorage()
	if err != nil {
		return nil, nil, err
	}
	authKey := traeFindAuthKey(root)
	providerID := strings.TrimPrefix(authKey, traeAuthKeyPrefix)
	authValue, err := traeDecodeStorageValue(root[authKey])
	if err != nil {
		return nil, nil, fmt.Errorf("could not decode Trae auth entry: %w", err)
	}
	authRaw, ok := authValue.(map[string]any)
	if !ok {
		return nil, nil, fmt.Errorf("Trae auth entry is not an object")
	}
	var serverRaw map[string]any
	if value, decodeErr := traeDecodeStorageValue(root[traeServerKeyPrefix+providerID]); decodeErr == nil {
		serverRaw, _ = value.(map[string]any)
	}
	accessToken := traeAuthTokenFromRaw(authRaw, serverRaw)
	if accessToken == "" {
		return nil, nil, fmt.Errorf("Trae storage.json does not contain an access token")
	}
	refreshToken := traeNestedString(authRaw,
		[]string{"refreshToken"}, []string{"refresh_token"}, []string{"RefreshToken"},
		[]string{"exchangeResponse", "Result", "RefreshToken"}, []string{"data", "refreshToken"})
	if refreshToken != "" {
		authRaw["refreshToken"] = refreshToken
	}
	email := traeNestedString(authRaw,
		[]string{"email"}, []string{"account", "email"}, []string{"data", "email"}, []string{"user", "email"})
	userID := traeNestedString(authRaw,
		[]string{"userId"}, []string{"uid"}, []string{"id"}, []string{"data", "userId"}, []string{"user", "id"})
	if userID == "" && serverRaw != nil {
		userID = traeNestedString(serverRaw, []string{"userId"}, []string{"uid"}, []string{"id"}, []string{"user", "id"})
	}
	creds := traeStoredCredentials{ProviderID: providerID, AuthKey: authKey, AuthRaw: authRaw}
	credentials, _ := json.MarshalIndent(creds, "", "  ")
	label := firstNonEmpty(email, userID, p.variant.name+" account")
	remoteKey := userID
	if remoteKey == "" {
		remoteKey = hashKey(accessToken)[:24]
	}
	account := &Account{
		ID:        makeAccountID(p.variant.id, remoteKey),
		Provider:  p.variant.id,
		Email:     email,
		RemoteID:  remoteKey,
		CreatedAt: nowMillis(),
		Label:     label,
	}
	return account, credentials, nil
}

func (p *traeProvider) RemoteKey(credentials []byte) (string, error) {
	creds, err := parseTraeCredentials(credentials)
	if err != nil {
		return "", err
	}
	userID := traeNestedString(creds.AuthRaw, []string{"userId"}, []string{"uid"}, []string{"id"})
	if userID != "" {
		return userID, nil
	}
	token := traeAuthTokenFromRaw(creds.AuthRaw, nil)
	return hashKey(token)[:24], nil
}

func (p *traeProvider) ActiveRemoteKey() (string, error) {
	root, _, err := p.readStorage()
	if err != nil {
		return "", err
	}
	authKey := traeFindAuthKey(root)
	authValue, err := traeDecodeStorageValue(root[authKey])
	if err != nil {
		return "", err
	}
	authRaw, _ := authValue.(map[string]any)
	if userID := traeNestedString(authRaw, []string{"userId"}, []string{"uid"}, []string{"id"}); userID != "" {
		return userID, nil
	}
	token := traeAuthTokenFromRaw(authRaw, nil)
	if token == "" {
		return "", fmt.Errorf("Trae auth entry has no identity")
	}
	return hashKey(token)[:24], nil
}

func (p *traeProvider) SwitchAccount(account *Account, credentials []byte) error {
	creds, err := parseTraeCredentials(credentials)
	if err != nil {
		return err
	}
	root, storePath, err := p.readStorage()
	if err != nil {
		return err
	}
	encoded, err := traeEncodeStorageValue(creds.AuthRaw)
	if err != nil {
		return err
	}
	root[creds.AuthKey] = encoded
	output, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(storePath, output, 0600)
}

func (p *traeProvider) RefreshQuota(ctx context.Context, account *Account, credentials []byte) (*Quota, []byte, error) {
	creds, err := parseTraeCredentials(credentials)
	if err != nil {
		return nil, nil, err
	}
	accessToken := traeAuthTokenFromRaw(creds.AuthRaw, nil)
	updated := credentials
	if accessToken != "" && traeTokenExpired(accessToken) {
		if refreshed, ok := p.refreshTokens(ctx, creds, accessToken); ok {
			accessToken = refreshed
			updated, _ = json.MarshalIndent(creds, "", "  ")
		}
	}
	if accessToken != "" {
		if quota, err := p.fetchAPIQuota(ctx, creds, accessToken); err == nil {
			return quota, updated, nil
		} else if refreshed, ok := p.refreshTokens(ctx, creds, accessToken); ok {
			accessToken = refreshed
			updated, _ = json.MarshalIndent(creds, "", "  ")
			if quota, retryErr := p.fetchAPIQuota(ctx, creds, accessToken); retryErr == nil {
				return quota, updated, nil
			}
		}
	}
	root, _, err := p.readStorage()
	if err != nil {
		return nil, updated, err
	}
	entitlementValue, decodeErr := traeDecodeStorageValue(root[traeEntitlementKeyPrefix+creds.ProviderID])
	if decodeErr != nil {
		return nil, updated, fmt.Errorf("Trae quota requires a refresh from the Trae API (token may be expired)")
	}
	quota := &Quota{UpdatedAt: nowMillis(), Allowed: true}
	walkTraeQuota(entitlementValue, quota)
	if len(quota.Metrics) == 0 {
		return nil, updated, fmt.Errorf("Trae quota requires a refresh from the Trae API (token may be expired)")
	}
	quota.Primary = &QuotaWindow{RemainingPercent: quota.Metrics[0].RemainingPercent, ResetAt: quota.Metrics[0].ResetAt}
	for _, metric := range quota.Metrics {
		if metric.RemainingPercent <= 0 {
			quota.LimitReached = true
		}
	}
	return quota, updated, nil
}

func traeTokenExpired(accessToken string) bool {
	claims := decodeJWTPayload(accessToken)
	if claims == nil {
		return false
	}
	exp, ok := jsonFloat(claims["exp"])
	if !ok {
		return false
	}
	return int64(exp) < time.Now().Unix()+120
}

func traeAuthClientID(variantID string, authRaw map[string]any) string {
	if clientID := traeNestedString(authRaw, []string{"authClientId"}, []string{"clientId"}); clientID != "" {
		return clientID
	}
	if variantID == "trae-solo" || variantID == "trae-solo-cn" {
		return "en1oxy7wnw8j9n"
	}
	return "ono9krqynydwx5"
}

func traeFindDeviceKeyPair(root map[string]any) (string, string) {
	for key, value := range root {
		if !strings.HasPrefix(key, traeDeviceKeyPrefix) {
			continue
		}
		decoded, err := traeDecodeStorageValue(value)
		if err != nil {
			continue
		}
		obj, ok := decoded.(map[string]any)
		if !ok {
			continue
		}
		privateKey := traeNestedString(obj, []string{"privateKeyPEM"}, []string{"private_key_pem"})
		publicKey := traeNestedString(obj, []string{"publicKeyPEM"}, []string{"public_key_pem"})
		if privateKey != "" && publicKey != "" {
			return privateKey, publicKey
		}
	}
	return "", ""
}

func traeSignDeviceProof(refreshToken string, privateKeyPEM string, clientID string) (map[string]any, error) {
	nonceBytes := make([]byte, 16)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, err
	}
	nonce := hex.EncodeToString(nonceBytes)
	timestamp := time.Now().Unix()
	message := strings.Join([]string{
		"POST",
		"/trae/api/v3/oauth/ExchangeToken",
		clientID,
		refreshToken,
		strconv.FormatInt(timestamp, 10),
		nonce,
	}, "\n")
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return nil, fmt.Errorf("invalid Trae device private key PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("error parsing Trae device private key: %w", err)
	}
	ecdsaKey, ok := parsed.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("Trae device key is not an ECDSA key")
	}
	digest := sha256.Sum256([]byte(message))
	signature, err := ecdsa.SignASN1(rand.Reader, ecdsaKey, digest[:])
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"Signature": base64.StdEncoding.EncodeToString(signature),
		"Timestamp": timestamp,
		"Nonce":     nonce,
	}, nil
}

func (p *traeProvider) refreshTokens(ctx context.Context, creds *traeStoredCredentials, accessToken string) (string, bool) {
	if creds.AuthRaw == nil || accessToken == "" {
		return "", false
	}
	refreshToken := traeNestedString(creds.AuthRaw,
		[]string{"refreshToken"}, []string{"refresh_token"}, []string{"RefreshToken"},
		[]string{"exchangeResponse", "Result", "RefreshToken"})
	if refreshToken == "" {
		return "", false
	}
	root, _, err := p.readStorage()
	if err != nil {
		return "", false
	}
	privateKey, publicKey := traeFindDeviceKeyPair(root)
	if privateKey == "" {
		return "", false
	}
	clientID := traeAuthClientID(p.variant.id, creds.AuthRaw)
	proof, err := traeSignDeviceProof(refreshToken, privateKey, clientID)
	if err != nil {
		return "", false
	}
	deviceInfo := map[string]any{}
	if existing, ok := creds.AuthRaw["deviceInfo"].(map[string]any); ok {
		for key, value := range existing {
			deviceInfo[key] = value
		}
	}
	deviceInfo["DevicePublicKey"] = publicKey
	if deviceInfo["PlatformCode"] == nil {
		deviceInfo["PlatformCode"] = "IDE_PC"
	}
	if deviceInfo["DeviceType"] == nil {
		deviceInfo["DeviceType"] = "PC"
	}
	if deviceInfo["ClientVersion"] == nil {
		deviceInfo["ClientVersion"] = firstNonEmpty(
			traeNestedString(creds.AuthRaw, []string{"deviceInfo", "ClientVersion"}),
			traeNestedString(creds.AuthRaw, []string{"ClientVersion"}),
			"3.5.66")
	}
	body := map[string]any{
		"ClientID":     clientID,
		"ClientSecret": "",
		"RefreshToken": refreshToken,
		"DeviceInfo":   deviceInfo,
		"DeviceProof":  proof,
		"IDEVersion":   deviceInfo["ClientVersion"],
	}
	paths := []string{"/trae/api/v3/oauth/ExchangeToken", "/cloudide/api/v3/trae/oauth/ExchangeToken"}
	for _, origin := range p.apiOrigins(creds) {
		for _, path := range paths {
			response, err := traePostJSON(ctx, origin+path, "Bearer "+accessToken, body)
			if err != nil {
				continue
			}
			root := response
			if data, ok := response["data"].(map[string]any); ok {
				root = data
			}
			newAccessToken := traeNestedString(root, []string{"Token"}, []string{"accessToken"}, []string{"access_token"}, []string{"token"})
			if newAccessToken == "" {
				continue
			}
			creds.AuthRaw["accessToken"] = newAccessToken
			creds.AuthRaw["token"] = newAccessToken
			if newRefreshToken := traeNestedString(root, []string{"RefreshToken"}, []string{"refresh_token"}); newRefreshToken != "" {
				creds.AuthRaw["refreshToken"] = newRefreshToken
			}
			return newAccessToken, true
		}
	}
	return "", false
}


func (p *traeProvider) apiOrigins(creds *traeStoredCredentials) []string {
	if p.variant.id == "trae-cn" || p.variant.id == "trae-solo-cn" {
		return []string{"https://api.trae.cn", "https://api.trae.com.cn"}
	}
	region := strings.ToUpper(traeNestedString(creds.AuthRaw,
		[]string{"storeRegion"}, []string{"storeCountryCode"}, []string{"userRegion", "region"}))
	origins := []string{}
	switch region {
	case "SG":
		origins = append(origins, "https://growsg-normal.trae.ai")
	case "US", "USTTP":
		origins = append(origins, "https://grow-normal.traeapi.us")
	}
	origins = append(origins, "https://grow-normal.trae.ai", "https://growsg-normal.trae.ai")
	if host := firstNonEmpty(traeNestedString(creds.AuthRaw, []string{"loginHost"}), traeNestedString(creds.AuthRaw, []string{"host"})); host != "" {
		origins = append(origins, strings.TrimRight(host, "/"))
	}
	return origins
}

func traePostJSON(ctx context.Context, url string, authHeader string, payload any) (map[string]any, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Trae/1.0.0 quasar")
	req.Header.Set("Authorization", authHeader)
	if strings.HasPrefix(authHeader, "Bearer ") {
		req.Header.Set("x-cloudide-token", strings.TrimPrefix(authHeader, "Bearer "))
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("Trae token rejected (%d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Trae API returned %d", resp.StatusCode)
	}
	var payloadMap map[string]any
	if err := json.Unmarshal(respBody, &payloadMap); err != nil {
		return nil, err
	}
	return payloadMap, nil
}

var traeProductTypePlan = map[float64]string{
	0: "Free", 1: "Lite", 4: "Pro", 5: "Pro+", 6: "Ultra", 8: "Trial", 9: "Solo Invite", 100: "CNExpress",
}

func traeUsageRoot(response map[string]any) map[string]any {
	for _, key := range []string{"user_current_entitlement_list", "ide_user_ent_usage", "data"} {
		if nested, ok := response[key].(map[string]any); ok {
			return nested
		}
	}
	return response
}

func (p *traeProvider) fetchAPIQuota(ctx context.Context, creds *traeStoredCredentials, accessToken string) (*Quota, error) {
	statusPath := "/trae/api/v1/pay/ide_user_pay_status"
	usagePath := "/trae/api/v1/pay/ide_user_ent_usage"
	isCN := p.variant.id == "trae-cn" || p.variant.id == "trae-solo-cn"
	if isCN {
		statusPath = "/trae/api/v2/pay/ide_user_pay_status"
		usagePath = "/trae/api/v2/pay/ide_user_ent_usage"
	}
	quota := &Quota{UpdatedAt: nowMillis(), Allowed: true}
	var usageResponse map[string]any
	found := false
	for _, origin := range p.apiOrigins(creds) {
		if response, err := traePostJSON(ctx, origin+statusPath, "Cloud-IDE-JWT "+accessToken, map[string]any{}); err == nil {
			traeApplyEntitlement(quota, response)
			found = true
		}
		if response, err := traePostJSON(ctx, origin+usagePath, "Cloud-IDE-JWT "+accessToken, map[string]any{}); err == nil {
			usageResponse = response
			found = true
			break
		}
		if response, err := traePostJSON(ctx, origin+usagePath, "Bearer "+accessToken, map[string]any{}); err == nil {
			usageResponse = response
			found = true
			break
		}
	}
	if usageResponse != nil {
		traeApplyUsage(quota, usageResponse, isCN)
	}
	if !found {
		return nil, fmt.Errorf("Trae API did not return quota data")
	}
	if len(quota.Metrics) == 0 {
		quota.Metrics = append(quota.Metrics, QuotaMetric{Name: "Credits", RemainingPercent: 100})
	}
	for _, metric := range quota.Metrics {
		if metric.RemainingPercent <= 0 {
			quota.LimitReached = true
		}
	}
	quota.Primary = &QuotaWindow{RemainingPercent: quota.Metrics[0].RemainingPercent, ResetAt: quota.Metrics[0].ResetAt}
	return quota, nil
}

func traeApplyEntitlement(quota *Quota, response map[string]any) {
	root := traeUsageRoot(response)
	packs, _ := root["user_entitlement_pack_list"].([]any)
	for _, entry := range packs {
		pack, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		base, _ := pack["entitlement_base_info"].(map[string]any)
		if base == nil {
			continue
		}
		if productType, ok := jsonFloat(base["product_type"]); ok {
			if plan, found := traeProductTypePlan[productType]; found {
				quota.PlanType = plan
			}
		}
	}
}

func traeApplyUsage(quota *Quota, response map[string]any, isCN bool) {
	root := traeUsageRoot(response)
	packs, _ := root["user_entitlement_pack_list"].([]any)
	preferred := []float64{100, 6, 5, 4, 1, 9, 8, 0}
	if !isCN {
		preferred = []float64{6, 4, 1, 9, 8, 0}
	}
	for _, productType := range preferred {
		for _, entry := range packs {
			pack, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			base, _ := pack["entitlement_base_info"].(map[string]any)
			if base == nil {
				continue
			}
			current, okCurrent := jsonFloat(base["product_type"])
			if !okCurrent || current != productType {
				continue
			}
			if plan, found := traeProductTypePlan[productType]; found {
				quota.PlanType = plan
			}
			resetAt := int64(0)
			if endTime, okEnd := jsonFloat(base["end_time"]); okEnd && endTime > 0 {
				resetAt = int64(endTime)
			}
			metrics := traeExtractMetrics(pack, resetAt)
			if len(metrics) > 0 {
				quota.Metrics = append(quota.Metrics, metrics...)
				return
			}
		}
	}
}

func traeExtractMetrics(value any, resetAt int64) []QuotaMetric {
	obj, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	lower := map[string]any{}
	for key, entry := range obj {
		lower[strings.ToLower(key)] = entry
	}
	used, hasUsed := jsonFloat(lower["used"])
	total, hasTotal := jsonFloat(lower["total"])
	if !hasTotal {
		if limit, okLimit := jsonFloat(lower["total_amount"]); okLimit {
			total = limit
			hasTotal = true
		}
	}
	if hasUsed && hasTotal && total > 0 {
		remaining := ((total - used) / total) * 100
		return []QuotaMetric{{Name: "Credits", RemainingPercent: normalizePercent(remaining), ResetAt: resetAt}}
	}
	if remaining, okRemaining := jsonFloat(lower["remaining"]); okRemaining && hasTotal && total > 0 {
		return []QuotaMetric{{Name: "Credits", RemainingPercent: normalizePercent((remaining / total) * 100), ResetAt: resetAt}}
	}
	var metrics []QuotaMetric
	for _, key := range []string{"usage", "detail", "entitlement_detail"} {
		if nested, found := lower[key]; found {
			metrics = append(metrics, traeExtractMetrics(nested, resetAt)...)
		}
	}
	return metrics
}

func walkTraeQuota(value any, quota *Quota) {
	obj, ok := value.(map[string]any)
	if !ok {
		return
	}
	lower := map[string]any{}
	for key, entry := range obj {
		lower[strings.ToLower(key)] = entry
	}
	total, hasTotal := jsonFloat(lower["total"])
	used, hasUsed := jsonFloat(lower["used"])
	remaining, hasRemaining := jsonFloat(lower["remaining"])
	if !hasRemaining {
		if limit, okLimit := jsonFloat(lower["total_credit"]); okLimit && limit > 0 {
			if usedCredit, okUsed := jsonFloat(lower["used_credit"]); okUsed {
				remaining = limit - usedCredit
				hasRemaining = true
			}
		}
	}
	if !hasTotal {
		for key, entry := range lower {
			if strings.Contains(key, "total") {
				if f, ok := jsonFloat(entry); ok {
					total = f
					hasTotal = true
					break
				}
			}
		}
	}
	if hasTotal && total > 0 && (hasRemaining || hasUsed) {
		remain := remaining
		if !hasRemaining {
			remain = total - used
		}
		name := firstNonEmpty(jsonString(lower["product_name"]), jsonString(lower["name"]), "Credits")
		quota.Metrics = append(quota.Metrics, QuotaMetric{
			Name:             name,
			RemainingPercent: normalizePercent((remain / total) * 100),
		})
		return
	}
	for _, key := range []string{"entitlementinfo", "detail", "data", "entitlementinfo_detail"} {
		if nested, found := lower[key]; found {
			walkTraeQuota(nested, quota)
		}
	}
}
