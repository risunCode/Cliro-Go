package kiro

import (
	"cliro-go/internal/util"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const socialCallbackPort = 9876

func (s *Service) startSocialCallbackServer(ctx context.Context, expectedState string) (string, <-chan SocialCallbackResult, error) {
	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", socialCallbackPort))
	if err != nil {
		listener, err = net.Listen("tcp", "localhost:0")
		if err != nil {
			return "", nil, fmt.Errorf("failed to start Kiro social callback server: %w", err)
		}
	}

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://localhost:%d/oauth/callback", port)
	resultCh := make(chan SocialCallbackResult, 1)
	server := &http.Server{ReadHeaderTimeout: 10 * time.Second}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		code := strings.TrimSpace(r.URL.Query().Get("code"))
		state := strings.TrimSpace(r.URL.Query().Get("state"))
		errParam := strings.TrimSpace(r.URL.Query().Get("error"))

		writeResult := func(result SocialCallbackResult, status int, title string, body string) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(status)
			_, _ = fmt.Fprintf(w, "<!DOCTYPE html><html><head><title>%s</title></head><body><h1>%s</h1><p>%s</p><p>You can close this window.</p></body></html>", html.EscapeString(title), html.EscapeString(title), html.EscapeString(body))
			select {
			case resultCh <- result:
			default:
			}
			go func() {
				_ = server.Shutdown(context.Background())
			}()
		}

		if errParam != "" {
			writeResult(SocialCallbackResult{Error: errParam}, http.StatusBadRequest, "Login Failed", errParam)
			return
		}
		if state != strings.TrimSpace(expectedState) {
			writeResult(SocialCallbackResult{Error: "state mismatch"}, http.StatusBadRequest, "Login Failed", "Invalid state parameter")
			return
		}
		writeResult(SocialCallbackResult{Code: code, State: state}, http.StatusOK, "Login Successful", "Return to CLIro-Go to finish connecting your Kiro account.")
	})
	server.Handler = mux

	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			select {
			case resultCh <- SocialCallbackResult{Error: err.Error()}:
			default:
			}
		}
	}()

	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
	}()

	return redirectURI, resultCh, nil
}

func (s *Service) exchangeSocialCode(ctx context.Context, code string, codeVerifier string) (*TokenData, error) {
	payload := map[string]string{
		"code":          strings.TrimSpace(code),
		"code_verifier": strings.TrimSpace(codeVerifier),
		"redirect_uri":  "kiro://kiro.kiroAgent/authenticate-success",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, SocialAuthURL+"/oauth/token", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", BuildSocialUserAgent())

	resp, err := s.client().Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("kiro social token exchange failed (%d): %s", resp.StatusCode, compactHTTPBody(respBody))
	}

	var parsed SocialTokenResponse
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}

	token := &TokenData{
		AccessToken:  strings.TrimSpace(parsed.AccessToken),
		RefreshToken: strings.TrimSpace(parsed.RefreshToken),
		ProfileARN:   strings.TrimSpace(parsed.ProfileARN),
		ExpiresIn:    parsed.ExpiresIn,
		TokenType:    "Bearer",
	}
	if token.ExpiresIn <= 0 {
		token.ExpiresIn = 3600
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

func (s *Service) refreshSocialToken(ctx context.Context, refreshToken string) (*TokenData, error) {
	trimmedRefreshToken := strings.TrimSpace(refreshToken)
	if trimmedRefreshToken == "" {
		return nil, fmt.Errorf("kiro social refresh token is empty")
	}

	payload := map[string]string{"refreshToken": trimmedRefreshToken}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, SocialAuthURL+"/refreshToken", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", BuildSocialUserAgent())

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
		return nil, fmt.Errorf("kiro social token refresh failed (%d): %s", resp.StatusCode, compactHTTPBody(respBody))
	}

	var parsed struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
		ProfileARN   string `json:"profileArn"`
		ExpiresIn    int    `json:"expiresIn"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, err
	}
	if strings.TrimSpace(parsed.AccessToken) == "" {
		return nil, fmt.Errorf("kiro social token refresh returned empty access token")
	}

	token := &TokenData{
		AccessToken:  strings.TrimSpace(parsed.AccessToken),
		RefreshToken: util.FirstNonEmpty(strings.TrimSpace(parsed.RefreshToken), trimmedRefreshToken),
		ExpiresIn:    parsed.ExpiresIn,
		TokenType:    "Bearer",
		ProfileARN:   strings.TrimSpace(parsed.ProfileARN),
		Email:        ExtractEmailFromJWT(parsed.AccessToken),
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
