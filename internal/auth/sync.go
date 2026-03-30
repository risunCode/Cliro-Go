package auth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cliro-go/internal/config"
)

const (
	oauthAuthKey          = "openai"
	kiloAuthTypeValue     = "openai"
	opencodeAuthTypeValue = "oauth"
)

var kiloAuthUpdatedFields = []string{
	"openai.type",
	"openai.access",
	"openai.refresh",
	"openai.expires",
	"openai.accountId",
}

var opencodeAuthUpdatedFields = []string{
	"openai.type",
	"openai.access",
	"openai.refresh",
	"openai.expires",
	"openai.accountId",
}

var codexAuthUpdatedFields = []string{
	"tokens.id_token",
	"tokens.access_token",
	"tokens.refresh_token",
	"tokens.account_id",
	"last_refresh",
}

type KiloAuthSyncResult struct {
	TargetPath      string   `json:"targetPath"`
	FileExisted     bool     `json:"fileExisted"`
	OpenAICreated   bool     `json:"openAICreated"`
	UpdatedFields   []string `json:"updatedFields"`
	AccountID       string   `json:"accountID"`
	Provider        string   `json:"provider"`
	SyncedExpires   int64    `json:"syncedExpires"`
	SyncedExpiresAt string   `json:"syncedExpiresAt,omitempty"`
}

type OpencodeAuthSyncResult struct {
	TargetPath      string   `json:"targetPath"`
	FileExisted     bool     `json:"fileExisted"`
	OpenAICreated   bool     `json:"openAICreated"`
	UpdatedFields   []string `json:"updatedFields"`
	AccountID       string   `json:"accountID"`
	Provider        string   `json:"provider"`
	SyncedExpires   int64    `json:"syncedExpires"`
	SyncedExpiresAt string   `json:"syncedExpiresAt,omitempty"`
}

type CodexAuthSyncResult struct {
	TargetPath    string   `json:"targetPath"`
	BackupPath    string   `json:"backupPath,omitempty"`
	FileExisted   bool     `json:"fileExisted"`
	BackupCreated bool     `json:"backupCreated"`
	UpdatedFields []string `json:"updatedFields"`
	AccountID     string   `json:"accountID"`
	Provider      string   `json:"provider"`
	SyncedAt      string   `json:"syncedAt"`
}

type kiloJSONField struct {
	Key   string
	Value json.RawMessage
}

type kiloJSONObject []kiloJSONField

type codexCLIAuthFile struct {
	AuthMode     string                 `json:"auth_mode"`
	OpenAIAPIKey any                    `json:"OPENAI_API_KEY"`
	Tokens       codexCLITokens         `json:"tokens"`
	LastRefresh  string                 `json:"last_refresh"`
	AccountID    string                 `json:"account_id,omitempty"`
	ExtraFields  map[string]interface{} `json:"-"`
}

type codexCLITokens struct {
	IDToken      string `json:"id_token"`
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	AccountID    string `json:"account_id,omitempty"`
}

func (m *Manager) SyncCodexAccountToKiloAuth(accountID string) (KiloAuthSyncResult, error) {
	account, err := m.findCodexAccountForSync(accountID, "Kilo CLI auth.json")
	if err != nil {
		return KiloAuthSyncResult{}, err
	}

	targetPath, err := kiloAuthFilePath()
	if err != nil {
		return KiloAuthSyncResult{}, err
	}

	root, fileExisted, err := loadOAuthAuthRoot(targetPath, "Kilo auth.json")
	if err != nil {
		return KiloAuthSyncResult{}, err
	}

	syncedExpires, syncedExpiresAt := kiloExpiryValue(account.ExpiresAt)
	updatedOpenAI, openAICreated, err := mergeOAuthOpenAIObject(root.openAIValue(), account, syncedExpires, kiloAuthTypeValue, false, "Kilo auth.json")
	if err != nil {
		return KiloAuthSyncResult{}, err
	}
	root = root.set(oauthAuthKey, updatedOpenAI)

	data, err := marshalKiloAuthJSON(root)
	if err != nil {
		return KiloAuthSyncResult{}, err
	}
	if err := writeRestrictedFile(targetPath, data, 0o600); err != nil {
		return KiloAuthSyncResult{}, err
	}

	return KiloAuthSyncResult{
		TargetPath:      targetPath,
		FileExisted:     fileExisted,
		OpenAICreated:   openAICreated,
		UpdatedFields:   append([]string(nil), kiloAuthUpdatedFields...),
		AccountID:       account.ID,
		Provider:        account.Provider,
		SyncedExpires:   syncedExpires,
		SyncedExpiresAt: syncedExpiresAt,
	}, nil
}

func (m *Manager) SyncCodexAccountToOpencodeAuth(accountID string) (OpencodeAuthSyncResult, error) {
	account, err := m.findCodexAccountForSync(accountID, "Opencode CLI auth.json")
	if err != nil {
		return OpencodeAuthSyncResult{}, err
	}

	targetPath, err := opencodeAuthFilePath()
	if err != nil {
		return OpencodeAuthSyncResult{}, err
	}

	root, fileExisted, err := loadOAuthAuthRoot(targetPath, "Opencode auth.json")
	if err != nil {
		return OpencodeAuthSyncResult{}, err
	}

	syncedExpires, syncedExpiresAt := kiloExpiryValue(account.ExpiresAt)
	updatedOpenAI, openAICreated, err := mergeOAuthOpenAIObject(root.openAIValue(), account, syncedExpires, opencodeAuthTypeValue, true, "Opencode auth.json")
	if err != nil {
		return OpencodeAuthSyncResult{}, err
	}
	root = root.set(oauthAuthKey, updatedOpenAI)

	data, err := marshalKiloAuthJSON(root)
	if err != nil {
		return OpencodeAuthSyncResult{}, err
	}
	if err := writeRestrictedFile(targetPath, data, 0o600); err != nil {
		return OpencodeAuthSyncResult{}, err
	}

	return OpencodeAuthSyncResult{
		TargetPath:      targetPath,
		FileExisted:     fileExisted,
		OpenAICreated:   openAICreated,
		UpdatedFields:   append([]string(nil), opencodeAuthUpdatedFields...),
		AccountID:       account.ID,
		Provider:        account.Provider,
		SyncedExpires:   syncedExpires,
		SyncedExpiresAt: syncedExpiresAt,
	}, nil
}

func (m *Manager) SyncCodexAccountToCodexCLI(accountID string) (CodexAuthSyncResult, error) {
	account, err := m.findCodexAccountForSync(accountID, "Codex CLI auth.json")
	if err != nil {
		return CodexAuthSyncResult{}, err
	}

	targetPath, err := codexAuthFilePath()
	if err != nil {
		return CodexAuthSyncResult{}, err
	}

	backupPath, backupCreated, err := createCodexAuthBackup(targetPath)
	if err != nil {
		return CodexAuthSyncResult{}, fmt.Errorf("failed to create backup: %w", err)
	}

	authFile, fileExisted, err := loadCodexAuthFile(targetPath)
	if err != nil {
		return CodexAuthSyncResult{}, err
	}

	authFile.Tokens.IDToken = strings.TrimSpace(account.IDToken)
	authFile.Tokens.AccessToken = strings.TrimSpace(account.AccessToken)
	authFile.Tokens.RefreshToken = strings.TrimSpace(account.RefreshToken)
	authFile.Tokens.AccountID = strings.TrimSpace(account.AccountID)
	authFile.LastRefresh = time.Now().UTC().Format(time.RFC3339Nano)
	if authFile.AccountID == "" && authFile.Tokens.AccountID != "" {
		authFile.AccountID = authFile.Tokens.AccountID
	}

	if err := saveCodexAuthFile(targetPath, authFile); err != nil {
		return CodexAuthSyncResult{}, err
	}

	return CodexAuthSyncResult{
		TargetPath:    targetPath,
		BackupPath:    backupPath,
		FileExisted:   fileExisted,
		BackupCreated: backupCreated,
		UpdatedFields: append([]string(nil), codexAuthUpdatedFields...),
		AccountID:     account.ID,
		Provider:      account.Provider,
		SyncedAt:      time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (m *Manager) findCodexAccountForSync(accountID, targetName string) (config.Account, error) {
	accountID = strings.TrimSpace(accountID)
	if accountID == "" {
		return config.Account{}, fmt.Errorf("account id is required")
	}

	account, ok := m.store.GetAccount(accountID)
	if !ok {
		return config.Account{}, fmt.Errorf("account not found: %s", accountID)
	}

	provider := strings.TrimSpace(strings.ToLower(account.Provider))
	if provider == "" {
		return config.Account{}, fmt.Errorf("account provider is required for sync to %s", targetName)
	}
	if provider != "codex" {
		return config.Account{}, fmt.Errorf("sync to %s only supports provider codex", targetName)
	}

	return account, nil
}

func kiloExpiryValue(raw int64) (int64, string) {
	if raw <= 0 {
		return 0, ""
	}

	var parsed time.Time
	if raw > 1_000_000_000_000 {
		parsed = time.UnixMilli(raw)
	} else {
		parsed = time.Unix(raw, 0)
	}

	parsed = parsed.UTC()
	return parsed.UnixMilli(), parsed.Format(time.RFC3339Nano)
}

func kiloAuthFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	if strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("resolve user home directory: empty path")
	}
	return filepath.Join(home, ".local", "share", "kilo", "auth.json"), nil
}

func opencodeAuthFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	if strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("resolve user home directory: empty path")
	}
	return filepath.Join(home, ".local", "share", "opencode", "auth.json"), nil
}

func loadOAuthAuthRoot(path, fileLabel string) (kiloJSONObject, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}

	root, err := parseOrderedJSONObject(data, "root", fileLabel)
	if err != nil {
		return nil, true, err
	}
	if raw, ok := root.get(oauthAuthKey); ok && isJSONNull(raw) {
		return nil, true, fmt.Errorf("invalid %s: openai must be a JSON object", fileLabel)
	}

	return root, true, nil
}

func mergeOAuthOpenAIObject(
	raw json.RawMessage,
	account config.Account,
	syncedExpires int64,
	typeValue string,
	forceType bool,
	fileLabel string,
) (json.RawMessage, bool, error) {
	openAICreated := false
	entries := kiloJSONObject(nil)
	if raw == nil {
		openAICreated = true
	} else {
		var err error
		entries, err = parseOrderedJSONObject(raw, oauthAuthKey, fileLabel)
		if err != nil {
			return nil, false, err
		}
	}

	managedValues := map[string]json.RawMessage{
		"type":      mustMarshalJSON(typeValue),
		"access":    mustMarshalJSON(strings.TrimSpace(account.AccessToken)),
		"refresh":   mustMarshalJSON(strings.TrimSpace(account.RefreshToken)),
		"expires":   mustMarshalJSON(syncedExpires),
		"accountId": mustMarshalJSON(strings.TrimSpace(account.AccountID)),
	}
	seen := map[string]bool{}

	for i := range entries {
		switch entries[i].Key {
		case "type":
			seen["type"] = true
			if forceType || kiloTypeNeedsDefault(entries[i].Value) {
				entries[i].Value = managedValues["type"]
			}
		case "access", "refresh", "expires", "accountId":
			seen[entries[i].Key] = true
			entries[i].Value = managedValues[entries[i].Key]
		}
	}

	for _, key := range []string{"type", "access", "refresh", "expires", "accountId"} {
		if seen[key] {
			continue
		}
		entries = append(entries, kiloJSONField{Key: key, Value: managedValues[key]})
	}

	data, err := marshalOrderedJSONObject(entries, 1)
	if err != nil {
		return nil, false, err
	}
	return data, openAICreated, nil
}

func parseOrderedJSONObject(data []byte, label, fileLabel string) (kiloJSONObject, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))

	tok, err := decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", fileLabel, err)
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, fmt.Errorf("invalid %s: %s must be a JSON object", fileLabel, label)
	}

	var fields kiloJSONObject
	for decoder.More() {
		keyTok, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("invalid %s: %w", fileLabel, err)
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, fmt.Errorf("invalid %s: %s must be a JSON object", fileLabel, label)
		}

		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("invalid %s: %w", fileLabel, err)
		}
		fields = append(fields, kiloJSONField{Key: key, Value: append(json.RawMessage(nil), value...)})
	}

	tok, err = decoder.Token()
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", fileLabel, err)
	}
	delim, ok = tok.(json.Delim)
	if !ok || delim != '}' {
		return nil, fmt.Errorf("invalid %s: %s must be a JSON object", fileLabel, label)
	}

	if tok, err := decoder.Token(); err == nil {
		_ = tok
		return nil, fmt.Errorf("invalid %s: multiple JSON values", fileLabel)
	} else if err != io.EOF {
		return nil, fmt.Errorf("invalid %s: trailing content", fileLabel)
	}

	return fields, nil
}

func marshalKiloAuthJSON(root kiloJSONObject) ([]byte, error) {
	data, err := marshalOrderedJSONObject(root, 0)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func marshalOrderedJSONObject(obj kiloJSONObject, level int) ([]byte, error) {
	if len(obj) == 0 {
		return []byte("{}"), nil
	}

	indent := strings.Repeat("  ", level)
	childIndent := strings.Repeat("  ", level+1)

	var buf bytes.Buffer
	buf.WriteByte('{')
	buf.WriteByte('\n')
	for i, field := range obj {
		key, err := json.Marshal(field.Key)
		if err != nil {
			return nil, err
		}
		buf.WriteString(childIndent)
		buf.Write(key)
		buf.WriteString(": ")
		buf.Write(field.Value)
		if i < len(obj)-1 {
			buf.WriteByte(',')
		}
		buf.WriteByte('\n')
	}
	buf.WriteString(indent)
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func (obj kiloJSONObject) get(key string) (json.RawMessage, bool) {
	for _, field := range obj {
		if field.Key == key {
			return field.Value, true
		}
	}
	return nil, false
}

func (obj kiloJSONObject) openAIValue() json.RawMessage {
	raw, _ := obj.get(oauthAuthKey)
	return raw
}

func (obj kiloJSONObject) set(key string, value json.RawMessage) kiloJSONObject {
	for i := range obj {
		if obj[i].Key == key {
			obj[i].Value = value
			return obj
		}
	}
	return append(obj, kiloJSONField{Key: key, Value: value})
}

func kiloTypeNeedsDefault(raw json.RawMessage) bool {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return true
	}
	return strings.TrimSpace(value) == ""
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func codexAuthFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home directory: %w", err)
	}
	if strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("resolve user home directory: empty path")
	}
	return filepath.Join(home, ".codex", "auth.json"), nil
}

func createCodexAuthBackup(targetPath string) (string, bool, error) {
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		return "", false, nil
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		return "", false, fmt.Errorf("read target file: %w", err)
	}

	backupPath := targetPath + ".bak.cliro-go"
	if err := writeRestrictedFile(backupPath, data, 0o600); err != nil {
		return "", false, fmt.Errorf("write backup file: %w", err)
	}

	return backupPath, true, nil
}

func loadCodexAuthFile(path string) (*codexCLIAuthFile, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &codexCLIAuthFile{
				AuthMode:     "chatgpt",
				OpenAIAPIKey: nil,
				Tokens:       codexCLITokens{},
				ExtraFields:  make(map[string]interface{}),
			}, false, nil
		}
		return nil, false, fmt.Errorf("read auth file: %w", err)
	}

	var rawMap map[string]interface{}
	if err := json.Unmarshal(data, &rawMap); err != nil {
		return nil, true, fmt.Errorf("parse auth file: %w", err)
	}

	authFile := &codexCLIAuthFile{ExtraFields: make(map[string]interface{})}
	if authMode, ok := rawMap["auth_mode"].(string); ok {
		authFile.AuthMode = authMode
	} else {
		authFile.AuthMode = "chatgpt"
	}
	authFile.OpenAIAPIKey = rawMap["OPENAI_API_KEY"]
	if accountID, ok := rawMap["account_id"].(string); ok {
		authFile.AccountID = accountID
	}

	if tokensRaw, ok := rawMap["tokens"].(map[string]interface{}); ok {
		if idToken, ok := tokensRaw["id_token"].(string); ok {
			authFile.Tokens.IDToken = idToken
		}
		if accessToken, ok := tokensRaw["access_token"].(string); ok {
			authFile.Tokens.AccessToken = accessToken
		}
		if refreshToken, ok := tokensRaw["refresh_token"].(string); ok {
			authFile.Tokens.RefreshToken = refreshToken
		}
		if tokenAccountID, ok := tokensRaw["account_id"].(string); ok {
			authFile.Tokens.AccountID = tokenAccountID
		}
	}

	knownFields := map[string]bool{
		"auth_mode":      true,
		"OPENAI_API_KEY": true,
		"tokens":         true,
		"last_refresh":   true,
		"account_id":     true,
	}
	for key, value := range rawMap {
		if !knownFields[key] {
			authFile.ExtraFields[key] = value
		}
	}

	return authFile, true, nil
}

func saveCodexAuthFile(path string, authFile *codexCLIAuthFile) error {
	output := make(map[string]interface{})
	output["auth_mode"] = authFile.AuthMode
	output["OPENAI_API_KEY"] = authFile.OpenAIAPIKey
	output["tokens"] = map[string]interface{}{
		"id_token":      authFile.Tokens.IDToken,
		"access_token":  authFile.Tokens.AccessToken,
		"refresh_token": authFile.Tokens.RefreshToken,
		"account_id":    authFile.Tokens.AccountID,
	}
	output["last_refresh"] = authFile.LastRefresh
	if authFile.AccountID != "" {
		output["account_id"] = authFile.AccountID
	}
	for key, value := range authFile.ExtraFields {
		output[key] = value
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal auth file: %w", err)
	}
	data = append(data, '\n')

	if err := writeRestrictedFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write auth file: %w", err)
	}
	return nil
}

func writeRestrictedFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		return err
	}
	return nil
}

func mustMarshalJSON(value any) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}
