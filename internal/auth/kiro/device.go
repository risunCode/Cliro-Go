package kiro

import (
	"cliro-go/internal/util"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

func (s *Service) registerClient(ctx context.Context) (*ClientRegistrationResponse, error) {
	payload := map[string]any{
		"clientName": BuilderClientName,
		"clientType": "public",
		"scopes":     BuilderScopes,
		"grantTypes": []string{"urn:ietf:params:oauth:grant-type:device_code", "refresh_token"},
		"issuerUrl":  BuilderStartURL,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, RegisterClientURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	applyOIDCHeaders(req)

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("kiro client registration failed (%d): %s", resp.StatusCode, compactHTTPBody(respBody))
	}

	var parsed ClientRegistrationResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	if strings.TrimSpace(parsed.ClientID) == "" || strings.TrimSpace(parsed.ClientSecret) == "" {
		return nil, fmt.Errorf("kiro client registration returned incomplete credentials")
	}
	return &parsed, nil
}

func (s *Service) startDeviceAuthorization(ctx context.Context, clientID, clientSecret string) (*DeviceAuthorizationResponse, error) {
	payload := map[string]string{
		"clientId":     strings.TrimSpace(clientID),
		"clientSecret": strings.TrimSpace(clientSecret),
		"startUrl":     BuilderStartURL,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, DeviceAuthURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	applyOIDCHeaders(req)

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("kiro device authorization failed (%d): %s", resp.StatusCode, compactHTTPBody(respBody))
	}

	var parsed DeviceAuthorizationResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	if strings.TrimSpace(parsed.DeviceCode) == "" || strings.TrimSpace(parsed.UserCode) == "" {
		return nil, fmt.Errorf("kiro device authorization returned incomplete device code data")
	}
	return &parsed, nil
}

func (s *Service) pollDeviceToken(ctx context.Context, clientID, clientSecret, deviceCode string, intervalSeconds int) (*TokenData, error) {
	if intervalSeconds < minimumPollInterval {
		intervalSeconds = minimumPollInterval
	}

	deadline := time.Now().Add(defaultDeviceWait)
	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("kiro device authorization timed out")
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Duration(intervalSeconds) * time.Second):
		}

		payload := map[string]string{
			"clientId":     strings.TrimSpace(clientID),
			"clientSecret": strings.TrimSpace(clientSecret),
			"deviceCode":   strings.TrimSpace(deviceCode),
			"grantType":    "urn:ietf:params:oauth:grant-type:device_code",
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, DeviceTokenURL, bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		applyOIDCHeaders(req)

		resp, err := s.client().Do(req)
		if err != nil {
			return nil, err
		}

		respBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			return nil, readErr
		}

		var parsed DeviceTokenResponse
		if err := json.Unmarshal(respBody, &parsed); err != nil {
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil, err
			}
			return nil, fmt.Errorf("kiro device token request failed (%d): %s", resp.StatusCode, compactHTTPBody(respBody))
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 || strings.TrimSpace(parsed.Error) != "" {
			switch strings.TrimSpace(parsed.Error) {
			case "authorization_pending":
				continue
			case "slow_down":
				intervalSeconds += minimumPollInterval
				continue
			case "expired_token":
				return nil, fmt.Errorf("kiro device code expired")
			case "access_denied":
				return nil, fmt.Errorf("kiro device authorization denied")
			}

			message := strings.TrimSpace(parsed.ErrorDescription)
			if message == "" {
				message = compactHTTPBody(respBody)
			}
			return nil, fmt.Errorf("kiro device token request failed (%d): %s", resp.StatusCode, message)
		}

		token := &TokenData{
			AccessToken:  strings.TrimSpace(parsed.AccessToken),
			RefreshToken: strings.TrimSpace(parsed.RefreshToken),
			ExpiresIn:    parsed.ExpiresIn,
			TokenType:    util.FirstNonEmpty(strings.TrimSpace(parsed.TokenType), "Bearer"),
			ProfileARN:   strings.TrimSpace(parsed.ProfileARN),
			ClientID:     strings.TrimSpace(clientID),
			ClientSecret: strings.TrimSpace(clientSecret),
		}
		token.Email = ExtractEmailFromJWT(token.AccessToken)
		if strings.TrimSpace(token.ProfileARN) == "" {
			profileARN, profileEmail, profileErr := s.fetchProfile(ctx, token.AccessToken)
			if profileErr == nil {
				token.ProfileARN = util.FirstNonEmpty(strings.TrimSpace(token.ProfileARN), strings.TrimSpace(profileARN))
				token.Email = util.FirstNonEmpty(strings.TrimSpace(token.Email), strings.TrimSpace(profileEmail))
			}
		}
		if strings.TrimSpace(token.Email) == "" {
			if email, emailErr := s.fetchUserEmailWithToken(ctx, token.AccessToken); emailErr == nil {
				token.Email = strings.TrimSpace(email)
			}
		}
		return token, nil
	}
}

func (s *Service) refreshTokens(ctx context.Context, clientID, clientSecret, refreshToken string) (*TokenData, error) {
	if strings.TrimSpace(clientID) == "" || strings.TrimSpace(clientSecret) == "" {
		return s.refreshSocialToken(ctx, refreshToken)
	}

	payload := map[string]string{
		"clientId":     strings.TrimSpace(clientID),
		"clientSecret": strings.TrimSpace(clientSecret),
		"refreshToken": strings.TrimSpace(refreshToken),
		"grantType":    "refresh_token",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, DeviceTokenURL, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	applyOIDCHeaders(req)

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("kiro token refresh failed (%d): %s", resp.StatusCode, compactHTTPBody(respBody))
	}

	var parsed DeviceTokenResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	if strings.TrimSpace(parsed.AccessToken) == "" {
		return nil, fmt.Errorf("kiro token refresh returned empty access token")
	}

	token := &TokenData{
		AccessToken:  strings.TrimSpace(parsed.AccessToken),
		RefreshToken: util.FirstNonEmpty(strings.TrimSpace(parsed.RefreshToken), strings.TrimSpace(refreshToken)),
		ExpiresIn:    parsed.ExpiresIn,
		TokenType:    util.FirstNonEmpty(strings.TrimSpace(parsed.TokenType), "Bearer"),
		ProfileARN:   strings.TrimSpace(parsed.ProfileARN),
		Email:        ExtractEmailFromJWT(parsed.AccessToken),
		ClientID:     strings.TrimSpace(clientID),
		ClientSecret: strings.TrimSpace(clientSecret),
	}
	if token.ExpiresIn <= 0 {
		token.ExpiresIn = 3600
	}
	if strings.TrimSpace(token.ProfileARN) == "" {
		profileARN, profileEmail, profileErr := s.fetchProfile(ctx, token.AccessToken)
		if profileErr == nil {
			token.ProfileARN = util.FirstNonEmpty(strings.TrimSpace(token.ProfileARN), strings.TrimSpace(profileARN))
			token.Email = util.FirstNonEmpty(strings.TrimSpace(token.Email), strings.TrimSpace(profileEmail))
		}
	}
	if strings.TrimSpace(token.Email) == "" {
		if email, emailErr := s.fetchUserEmailWithToken(ctx, token.AccessToken); emailErr == nil {
			token.Email = strings.TrimSpace(email)
		}
	}
	return token, nil
}

func applyOIDCHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("x-amz-user-agent", "aws-sdk-js/1.0.27 KiroIDE")
	req.Header.Set("User-Agent", "aws-sdk-js/1.0.27 ua/2.1 os/linux lang/js md/nodejs#22.21.1 api/sso-oidc#1.0.27 m/E KiroIDE")
	req.Header.Set("amz-sdk-invocation-id", uuid.NewString())
	req.Header.Set("amz-sdk-request", "attempt=1; max=4")
}
