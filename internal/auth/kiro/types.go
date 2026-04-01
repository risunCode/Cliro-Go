package kiro

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/google/uuid"
)

const (
	RegisterClientURL   = "https://oidc.us-east-1.amazonaws.com/client/register"
	DeviceAuthURL       = "https://oidc.us-east-1.amazonaws.com/device_authorization"
	DeviceTokenURL      = "https://oidc.us-east-1.amazonaws.com/token"
	BuilderStartURL     = "https://view.awsapps.com/start"
	BuilderClientName   = "kiro-oauth-client"
	RuntimeUserAgent    = "aws-sdk-js/1.0.27 ua/2.1 os/linux lang/js md/nodejs#22.21.1 api/codewhispererstreaming#1.0.27 m/E KiroIDE-0.10.32"
	RuntimeAmzUserAgent = "aws-sdk-js/1.0.27 KiroIDE 0.10.32"
	SocialAuthURL       = "https://prod.us-east-1.auth.desktop.kiro.dev"
)

var BuilderScopes = []string{
	"codewhisperer:completions",
	"codewhisperer:analysis",
	"codewhisperer:conversations",
	"codewhisperer:transformations",
	"codewhisperer:taskassist",
}

type AuthStart struct {
	SessionID       string `json:"sessionId"`
	AuthURL         string `json:"authUrl"`
	VerificationURL string `json:"verificationUrl,omitempty"`
	UserCode        string `json:"userCode,omitempty"`
	ExpiresAt       int64  `json:"expiresAt,omitempty"`
	Status          string `json:"status"`
	AuthMethod      string `json:"authMethod,omitempty"`
	Provider        string `json:"provider,omitempty"`
}

type AuthSessionView struct {
	SessionID       string `json:"sessionId"`
	AuthURL         string `json:"authUrl"`
	VerificationURL string `json:"verificationUrl,omitempty"`
	UserCode        string `json:"userCode,omitempty"`
	ExpiresAt       int64  `json:"expiresAt,omitempty"`
	Status          string `json:"status"`
	Error           string `json:"error,omitempty"`
	AccountID       string `json:"accountId,omitempty"`
	Email           string `json:"email,omitempty"`
	AuthMethod      string `json:"authMethod,omitempty"`
	Provider        string `json:"provider,omitempty"`
}

type SocialCallbackResult struct {
	Code  string
	State string
	Error string
}

type SocialProvider string

const (
	SocialProviderGoogle SocialProvider = "Google"
	SocialProviderGitHub SocialProvider = "Github"
)

type ClientRegistrationResponse struct {
	ClientID              string `json:"clientId"`
	ClientSecret          string `json:"clientSecret"`
	ClientSecretExpiresAt int64  `json:"clientSecretExpiresAt"`
}

type DeviceAuthorizationResponse struct {
	DeviceCode              string `json:"deviceCode"`
	UserCode                string `json:"userCode"`
	VerificationURI         string `json:"verificationUri"`
	VerificationURIComplete string `json:"verificationUriComplete"`
	ExpiresIn               int    `json:"expiresIn"`
	Interval                int    `json:"interval"`
}

type DeviceTokenResponse struct {
	AccessToken      string `json:"accessToken"`
	RefreshToken     string `json:"refreshToken"`
	ExpiresIn        int    `json:"expiresIn"`
	TokenType        string `json:"tokenType"`
	ProfileARN       string `json:"profileArn"`
	Error            string `json:"error"`
	ErrorDescription string `json:"error_description"`
}

type TokenData struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	TokenType    string
	ProfileARN   string
	Email        string
	ClientID     string
	ClientSecret string
}

type SocialTokenResponse struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	ProfileARN   string `json:"profileArn"`
	ExpiresIn    int    `json:"expiresIn"`
}

func NormalizeSocialProvider(provider string) (SocialProvider, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "google", "":
		return SocialProviderGoogle, nil
	case "github":
		return SocialProviderGitHub, nil
	default:
		return "", fmt.Errorf("unsupported Kiro social provider: %s", strings.TrimSpace(provider))
	}
}

func GeneratePKCE() (string, string, error) {
	raw := make([]byte, 64)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	verifier := base64.RawURLEncoding.EncodeToString(raw)
	if len(verifier) > 128 {
		verifier = verifier[:128]
	}
	hashed := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hashed[:])
	return verifier, challenge, nil
}

func GenerateState() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func BuildSocialLoginURL(provider SocialProvider, codeChallenge string, state string) string {
	customRedirectURI := "kiro://kiro.kiroAgent/authenticate-success"
	return fmt.Sprintf("%s/login?idp=%s&redirect_uri=%s&code_challenge=%s&code_challenge_method=S256&state=%s",
		SocialAuthURL,
		url.QueryEscape(string(provider)),
		url.QueryEscape(customRedirectURI),
		url.QueryEscape(strings.TrimSpace(codeChallenge)),
		url.QueryEscape(strings.TrimSpace(state)),
	)
}

func BuildSocialUserAgent() string {
	return fmt.Sprintf("KiroIDE-0.10.32-%s", strings.ReplaceAll(uuid.NewString(), "-", ""))
}

func DetermineAuthMethod(tokens *TokenData) string {
	if tokens == nil {
		return "social"
	}
	if strings.TrimSpace(tokens.ClientID) != "" && strings.TrimSpace(tokens.ClientSecret) != "" {
		return "idc"
	}
	return "social"
}

func ExtractEmailFromJWT(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	return strings.TrimSpace(claims.Email)
}
