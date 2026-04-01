package kiro

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const quotaBaseURL = "https://codewhisperer.us-east-1.amazonaws.com"

func applyRuntimeHeaders(req *http.Request, accessToken string) {
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(accessToken))
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", RuntimeUserAgent)
	req.Header.Set("x-amz-user-agent", RuntimeAmzUserAgent)
	req.Header.Set("x-amzn-codewhisperer-optout", "true")
}

func (s *Service) fetchProfile(ctx context.Context, accessToken string) (string, string, error) {
	body := []byte(`{"origin":"AI_EDITOR"}`)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, quotaBaseURL, bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.0")
	req.Header.Set("X-Amz-Target", "AmazonCodeWhispererService.ListAvailableModels")
	applyRuntimeHeaders(req, accessToken)

	resp, err := s.client().Do(req)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("kiro profile fetch failed (%d): %s", resp.StatusCode, compactHTTPBody(respBody))
	}

	var data struct {
		ProfileARN string `json:"profileArn"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return "", "", err
	}
	return strings.TrimSpace(data.ProfileARN), ExtractEmailFromJWT(accessToken), nil
}

func (s *Service) fetchUserEmailWithToken(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, quotaBaseURL+"/GetUserInfo", strings.NewReader(`{"origin":"KIRO_IDE"}`))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	applyRuntimeHeaders(req, accessToken)

	resp, err := s.client().Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("kiro GetUserInfo failed (%d): %s", resp.StatusCode, compactHTTPBody(respBody))
	}

	var data struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(respBody, &data); err != nil {
		return "", err
	}
	return strings.TrimSpace(data.Email), nil
}

func compactHTTPBody(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return "empty response"
	}
	if len(trimmed) > 180 {
		return trimmed[:180] + "..."
	}
	return trimmed
}
