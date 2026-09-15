package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

const e2eMaxResponseBytes = 1 << 20
const e2eHTTPTimeout = 15 * time.Second

type e2eHTTPClient struct {
	baseURL    *url.URL
	httpClient *http.Client
}

type e2eHTTPResponse struct {
	StatusCode  int
	Location    string
	Body        []byte
	CookieNames []string
}

func newE2EHTTPClient(baseURL string) (*e2eHTTPClient, error) {
	parsed, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil {
		return nil, errors.New("base URL must be an absolute origin without a path")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, errors.New("base URL scheme must be http or https")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("create cookie jar: %w", err)
	}
	return &e2eHTTPClient{
		baseURL: parsed,
		httpClient: &http.Client{
			Jar:       jar,
			Timeout:   e2eHTTPTimeout,
			Transport: http.DefaultTransport,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}, nil
}

func (c *e2eHTTPClient) endpoint(path string) (*url.URL, error) {
	if c == nil || c.baseURL == nil {
		return nil, errors.New("HTTP client is not initialized")
	}
	parsed, err := url.Parse(path)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") {
		return nil, errors.New("request path must be relative to the configured origin")
	}
	resolved := c.baseURL.ResolveReference(parsed)
	if resolved.Scheme != c.baseURL.Scheme || resolved.Host != c.baseURL.Host {
		return nil, errors.New("request path escaped the configured origin")
	}
	return resolved, nil
}

func (c *e2eHTTPClient) do(ctx context.Context, method, path string, form url.Values) (e2eHTTPResponse, error) {
	endpoint, err := c.endpoint(path)
	if err != nil {
		return e2eHTTPResponse{}, err
	}
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return e2eHTTPResponse{}, fmt.Errorf("create HTTP request: %w", err)
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	response, err := c.httpClient.Do(req)
	if err != nil {
		return e2eHTTPResponse{}, fmt.Errorf("HTTP %s %s: %w", method, endpoint.Path, err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, e2eMaxResponseBytes+1))
	if err != nil {
		return e2eHTTPResponse{}, fmt.Errorf("read HTTP %s %s response: %w", method, endpoint.Path, err)
	}
	if len(responseBody) > e2eMaxResponseBytes {
		return e2eHTTPResponse{}, fmt.Errorf("HTTP %s %s response exceeded %d bytes", method, endpoint.Path, e2eMaxResponseBytes)
	}
	cookieNames := make([]string, 0, len(response.Cookies()))
	for _, cookie := range response.Cookies() {
		cookieNames = append(cookieNames, cookie.Name)
	}
	return e2eHTTPResponse{
		StatusCode:  response.StatusCode,
		Location:    response.Header.Get("Location"),
		Body:        responseBody,
		CookieNames: cookieNames,
	}, nil
}

func e2eHTTPResponseSummary(response e2eHTTPResponse) map[string]any {
	return map[string]any{
		"status_code":        response.StatusCode,
		"location":           response.Location,
		"session_cookie_set": containsString(response.CookieNames, "rentops_session"),
	}
}

func e2eHTTPStep(method, path string, expected any, response e2eHTTPResponse, err error) e2eStepReport {
	step := e2eStepReport{
		Method:     method,
		Path:       path,
		StatusCode: response.StatusCode,
		Expected:   expected,
		Actual:     e2eHTTPResponseSummary(response),
		Passed:     err == nil,
	}
	if err != nil {
		step.Passed = false
		step.Error = err.Error()
	}
	return step
}

func (c *e2eHTTPClient) authenticationScenario(ctx context.Context, username, password string) e2eScenarioReport {
	scenario := e2eScenarioReport{Name: "authentication", Status: "running", Steps: []e2eStepReport{}}
	markFailure := func(message string) e2eScenarioReport {
		scenario.Status = "failed"
		scenario.Error = message
		return scenario
	}

	loginResponse, err := c.do(ctx, http.MethodPost, "/login-local", url.Values{
		"username": {username},
		"password": {password},
	})
	loginStep := e2eHTTPStep(http.MethodPost, "/login-local", map[string]any{
		"status_code":        http.StatusFound,
		"location":           "/rent-dashboard",
		"session_cookie_set": true,
	}, loginResponse, err)
	if err == nil {
		loginStep.Passed = loginResponse.StatusCode == http.StatusFound && loginResponse.Location == "/rent-dashboard" && containsString(loginResponse.CookieNames, "rentops_session")
		if !loginStep.Passed {
			loginStep.Error = "local login did not issue the expected redirect and session cookie"
		}
	}
	scenario.Steps = append(scenario.Steps, loginStep)
	if !loginStep.Passed {
		return markFailure("valid local login failed")
	}

	protectedResponse, err := c.do(ctx, http.MethodGet, "/billing", nil)
	protectedStep := e2eHTTPStep(http.MethodGet, "/billing", map[string]any{
		"status_code": http.StatusOK,
	}, protectedResponse, err)
	if err == nil {
		protectedStep.Passed = protectedResponse.StatusCode == http.StatusOK
		if !protectedStep.Passed {
			protectedStep.Error = "authenticated request did not reach the protected page"
		}
	}
	scenario.Steps = append(scenario.Steps, protectedStep)
	if !protectedStep.Passed {
		return markFailure("shared cookie session was not accepted by a protected endpoint")
	}

	unauthenticated, err := newE2EHTTPClient(c.baseURL.String())
	if err != nil {
		return markFailure("create isolated unauthenticated client: " + err.Error())
	}
	invalidLoginResponse, err := unauthenticated.do(ctx, http.MethodPost, "/login-local", url.Values{
		"username": {username},
		"password": {"invalid"},
	})
	invalidLoginStep := e2eHTTPStep(http.MethodPost, "/login-local", map[string]any{
		"status_code":        http.StatusFound,
		"location":           "/?error=invalid_login",
		"session_cookie_set": false,
	}, invalidLoginResponse, err)
	if err == nil {
		invalidLoginStep.Passed = invalidLoginResponse.StatusCode == http.StatusFound && invalidLoginResponse.Location == "/?error=invalid_login" && !containsString(invalidLoginResponse.CookieNames, "rentops_session")
		if !invalidLoginStep.Passed {
			invalidLoginStep.Error = "invalid local login was not rejected without a session cookie"
		}
	}
	scenario.Steps = append(scenario.Steps, invalidLoginStep)
	if !invalidLoginStep.Passed {
		return markFailure("invalid local login was accepted")
	}

	unauthorizedResponse, err := unauthenticated.do(ctx, http.MethodGet, "/billing", nil)
	unauthorizedStep := e2eHTTPStep(http.MethodGet, "/billing", map[string]any{
		"status_code": http.StatusFound,
		"location":    "/",
	}, unauthorizedResponse, err)
	if err == nil {
		unauthorizedStep.Passed = unauthorizedResponse.StatusCode == http.StatusFound && unauthorizedResponse.Location == "/"
		if !unauthorizedStep.Passed {
			unauthorizedStep.Error = "unauthenticated protected request was not redirected to login"
		}
	}
	scenario.Steps = append(scenario.Steps, unauthorizedStep)
	if !unauthorizedStep.Passed {
		return markFailure("unauthorized protected request was accepted")
	}

	scenario.Status = "passed"
	return scenario
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
