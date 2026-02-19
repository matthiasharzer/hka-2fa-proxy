package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/MatthiasHarzer/hka-2fa-proxy/otp"
)

// validAuthKey matches only safe characters for use in a URL path segment.
var validAuthKey = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// errUnauthorized is returned by proxyRequest when the request lacks a valid auth key.
// The HTTP 401 response has already been written to the client at that point.
var errUnauthorized = errors.New("unauthorized")

const (
	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36"
)

func isLoginSuccessful(response *http.Response) bool {
	isFound := response.StatusCode == http.StatusFound
	if !isFound {
		return false
	}

	location := response.Header.Get("Location")
	return location == "/"
}

func isLoginPage(body string) bool {
	return strings.Contains(body, "Welcome to HKA MFA-protected Services.")
}

func isRedirectToLogin(response *http.Response) bool {
	location := response.Header.Get("Location")
	return strings.Contains(location, "lm_auth_proxy")
}

type server struct {
	otpGenerator  otp.Generator
	targetBaseURL string
	username      string
	jar           *cookiejar.Jar
	jarMutex      *sync.RWMutex
	authKey       string
}

func NewServer(targetBaseURL, username string, otpGenerator otp.Generator, skipAuth bool, authKey string) (http.Handler, error) {
	if authKey != "" && !validAuthKey.MatchString(authKey) {
		return nil, fmt.Errorf("authKey must contain only alphanumeric characters, hyphens, and underscores")
	}
	sv := &server{
		targetBaseURL: targetBaseURL,
		otpGenerator:  otpGenerator,
		username:      username,
		jarMutex:      &sync.RWMutex{},
		authKey:       authKey,
	}
	if !skipAuth {
		err := sv.authenticateClient()
		if err != nil {
			return nil, fmt.Errorf("could not authenticate client: %w", err)
		}
	} else {
		err := sv.clearCookies()
		if err != nil {
			return nil, fmt.Errorf("could not clear cookies: %w", err)
		}
	}
	return sv, nil
}

// getLoginParameters performs the initial request to get session cookies and login form parameters.
func (s *server) getLoginParameters() (url.Values, string, error) {
	// Create a request so we can set headers
	initialURL := s.targetBaseURL + "/"
	req, err := http.NewRequest("GET", initialURL, nil)
	if err != nil {
		return nil, "", fmt.Errorf("could not create initial request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	// The client's CheckRedirect is configured to stop redirects
	resp, err := s.doRequest(req)
	if err != nil {
		return nil, "", fmt.Errorf("initial GET request failed: %w", err)
	}
	defer resp.Body.Close()

	// We expect a 302 Found status code
	if resp.StatusCode != http.StatusFound {
		return nil, "", fmt.Errorf("expected a 302 redirect, but got status %s", resp.Status)
	}

	locationHeader := resp.Header.Get("Location")
	if locationHeader == "" {
		return nil, "", fmt.Errorf("'Location' header not found in the response")
	}

	// Parse parameters from the unusual URL format (split by '?')
	parts := strings.Split(locationHeader, "?")
	if len(parts) < 2 {
		return nil, "", fmt.Errorf("could not parse query string from location: %s", locationHeader)
	}
	parsedParams, err := url.ParseQuery(parts[len(parts)-1])
	if err != nil {
		return nil, "", fmt.Errorf("could not parse query parameters: %w", err)
	}

	refererURL := s.targetBaseURL + locationHeader
	return parsedParams, refererURL, nil
}

// submitLogin prompts for credentials and submits the login form.
func (s *server) submitLogin(params url.Values, refererURL, username, password string) (*http.Response, error) {
	// Prepare form data for the POST request
	formData := url.Values{}
	formData.Set("curl", params.Get("curl"))
	formData.Set("curlid", params.Get("curlid"))
	formData.Set("curlmode", params.Get("curlmode"))
	formData.Set("username", strings.TrimSpace(username))
	formData.Set("password", password)

	postURL := fmt.Sprintf("%s/lm_auth_proxy?LMLogon", s.targetBaseURL)

	req, err := http.NewRequest("POST", postURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return nil, fmt.Errorf("could not create POST request: %w", err)
	}

	// Set necessary headers
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Referer", refererURL)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// Execute the request
	resp, err := s.doRequest(req)
	if err != nil {
		return nil, fmt.Errorf("login POST request failed: %w", err)
	}

	return resp, nil
}

func (s *server) clearCookies() error {
	s.jarMutex.Lock()
	defer s.jarMutex.Unlock()

	jar, err := cookiejar.New(nil)
	if err != nil {
		return fmt.Errorf("could not create cookie jar: %w", err)
	}
	s.jar = jar
	return nil
}

func (s *server) authenticateClient() error {
	log.Println("authenticating client")

	err := s.clearCookies()
	if err != nil {
		return fmt.Errorf("could not clear cookies: %w", err)
	}

	loginParams, refererURL, err := s.getLoginParameters()
	if err != nil {
		return fmt.Errorf("could not get login parameters: %w", err)
	}

	// This is required, as one OTP can only be used once. To prevent timing issues, we wait for the next interval.
	s.otpGenerator.WaitForNextInterval()
	password := s.otpGenerator.Generate(time.Now())

	loginResp, err := s.submitLogin(loginParams, refererURL, s.username, password)
	if err != nil {
		return fmt.Errorf("could not submit login: %w", err)
	}
	defer loginResp.Body.Close()

	if !isLoginSuccessful(loginResp) {
		return fmt.Errorf("login failed")
	}

	log.Println("client authenticated successfully")

	return nil
}

func (s *server) getHttpClient(url *url.URL) (*http.Client, error) {
	s.jarMutex.RLock()
	defer s.jarMutex.RUnlock()

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("could not create cookie jar: %w", err)
	}

	cookies := s.jar.Cookies(url)
	jar.SetCookies(url, cookies)

	client := &http.Client{
		Timeout: 10 * time.Second,
		Jar:     jar,
	}

	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		// This stops the client from following any redirects
		return http.ErrUseLastResponse
	}

	return client, nil
}

func (s *server) saveCookiesFromResponse(url *url.URL, resp *http.Response) {
	cookies := resp.Cookies()
	if len(cookies) == 0 {
		return
	}

	s.jarMutex.Lock()
	defer s.jarMutex.Unlock()

	for _, cookie := range cookies {
		s.jar.SetCookies(url, []*http.Cookie{cookie})
	}
}

func (s *server) doRequest(req *http.Request) (*http.Response, error) {
	client, err := s.getHttpClient(req.URL)
	if err != nil {
		return nil, fmt.Errorf("could not get HTTP client: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	s.saveCookiesFromResponse(req.URL, resp)

	return resp, nil
}

func (s *server) replaceHTMLContent(body string) string {
	newBody := body

	if s.authKey != "" {
		// qisserver forward
		newBody = strings.ReplaceAll(newBody, `URL=/`, fmt.Sprintf(`URL=/_/%s/`, s.authKey))

		// Quick and dirty way to rewrite URLs in form action attributes and links
		newBody = strings.ReplaceAll(newBody, `action="/`, fmt.Sprintf(`action="/_/%s/`, s.authKey))
		newBody = strings.ReplaceAll(newBody, `href="/`, fmt.Sprintf(`href="/_/%s/`, s.authKey))
		newBody = strings.ReplaceAll(newBody, `src="/`, fmt.Sprintf(`src="/_/%s/`, s.authKey))
		newBody = strings.ReplaceAll(newBody, fmt.Sprintf(`action="%s/`, s.targetBaseURL), fmt.Sprintf(`action="/_/%s/`, s.authKey))
		newBody = strings.ReplaceAll(newBody, fmt.Sprintf(`href="%s/`, s.targetBaseURL), fmt.Sprintf(`href="/_/%s/`, s.authKey))
		newBody = strings.ReplaceAll(newBody, fmt.Sprintf(`src="%s/`, s.targetBaseURL), fmt.Sprintf(`src="/_/%s/`, s.authKey))
	} else {
		newBody = strings.ReplaceAll(newBody, fmt.Sprintf(`action="%s/`, s.targetBaseURL), `action="/`)
		newBody = strings.ReplaceAll(newBody, fmt.Sprintf(`href="%s/`, s.targetBaseURL), `href="/`)
		newBody = strings.ReplaceAll(newBody, fmt.Sprintf(`src="%s/`, s.targetBaseURL), `src="/`)
	}

	return newBody
}

func (s *server) isAuthorizedRequest(r *http.Request) bool {
	if s.authKey == "" {
		return true
	}

	parts := strings.Split(r.URL.Path, "/")

	if len(parts) > 2 && parts[1] == "_" && parts[2] == s.authKey {
		return true
	}

	return false
}

func (s *server) resolveTargetRequestURI(r *http.Request) (string, error) {
	if s.authKey == "" {
		uri := r.URL.Path
		if r.URL.RawQuery != "" {
			uri = uri + "?" + r.URL.RawQuery
		}
		return uri, nil
	}
	parts := strings.Split(r.URL.Path, "/")

	if len(parts) == 3 && parts[1] == "_" && parts[2] == s.authKey {
		return "/", nil
	}
	if len(parts) > 3 && parts[1] == "_" && parts[2] == s.authKey {
		path := "/" + strings.Join(parts[3:], "/")
		if r.URL.RawQuery != "" {
			path = path + "?" + r.URL.RawQuery
		}
		return path, nil
	}

	return "", fmt.Errorf("invalid request URI: %s", r.RequestURI)
}

func (s *server) getLocation(location string) string {
	// If the location is an absolute URL under the target base URL, strip the base.
	if strings.HasPrefix(location, s.targetBaseURL) {
		trimmed := strings.TrimPrefix(location, s.targetBaseURL)
		if trimmed == "" {
			trimmed = "/"
		}
		if s.authKey == "" {
			return trimmed
		}
		return "/_/" + s.authKey + trimmed
	}

	// For relative paths (starting with '/'), rewrite them to pass through the proxy.
	if strings.HasPrefix(location, "/") {
		if s.authKey == "" {
			return location
		}
		return "/_/" + s.authKey + location
	}

	// For other absolute URLs or unusual values, return as-is to avoid creating malformed URLs.
	return location
}

func (s *server) proxyRequest(w http.ResponseWriter, r *http.Request) error {
	if !s.isAuthorizedRequest(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return errUnauthorized
	}
	requestURI, err := s.resolveTargetRequestURI(r)
	if err != nil {
		return fmt.Errorf("could not resolve target request URI: %w", err)
	}

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("could not read request body: %w", err)
	}
	bodyString := string(bodyBytes)

	// Create a new request based on the original one
	proxyReq, err := http.NewRequest(r.Method, s.targetBaseURL+requestURI, strings.NewReader(bodyString))
	if err != nil {
		return err
	}

	// Copy headers from the original request
	for name, values := range r.Header {
		for _, value := range values {
			if strings.ToLower(name) == "cookie" {
				continue
			}
			if strings.ToLower(name) == "referer" {
				continue
			}
			if strings.ToLower(name) == "accept-encoding" {
				continue
			}
			proxyReq.Header.Add(name, value)
		}
	}
	proxyReq.Header.Set("User-Agent", userAgent)
	proxyReq.Header.Set("Origin", s.targetBaseURL)

	// Perform the request
	resp, err := s.doRequest(proxyReq)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	responseBodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("error reading response body: %w", err)
	}
	responseBody := string(responseBodyBytes)

	contentType := resp.Header.Get("Content-Type")
	if strings.Contains(contentType, "text/html") {
		// Quick and dirty way to rewrite URLs in form action attributes and links
		responseBodyNew := s.replaceHTMLContent(responseBody)
		responseBodyBytes = []byte(responseBodyNew)
	}

	if isLoginPage(responseBody) || isRedirectToLogin(resp) {
		return fmt.Errorf("not logged in anymore")
	}

	// Copy response headers
	for name, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		location := resp.Header.Get("Location")
		location = s.getLocation(location)
		w.Header().Set("Location", location)
	}

	w.Header().Set("Content-Length", strconv.Itoa(len(responseBodyBytes)))

	// Write the status code
	w.WriteHeader(resp.StatusCode)

	// Copy the response body
	_, err = w.Write(responseBodyBytes)
	if err != nil {
		log.Printf("error copying response body: %v", err)
	}
	return nil
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("proxying request: %s %s", r.Method, r.URL.String())

	err := s.proxyRequest(w, r)
	if err != nil {
		log.Printf("error proxying request: %v", err)

		if errors.Is(err, errUnauthorized) {
			// Response already written; no re-authentication needed for wrong auth keys.
			return
		}

		if errors.Is(err, context.DeadlineExceeded) {
			http.Error(w, "request timed out: "+err.Error(), http.StatusGatewayTimeout)
			return
		}

		err = s.authenticateClient()
		if err != nil {
			log.Printf("re-authentication failed: %v", err)
			http.Error(w, "re-authentication failed: "+err.Error(), http.StatusBadGateway)
			return
		}
		err = s.proxyRequest(w, r)
		if err != nil {
			log.Printf("proxy error after re-authentication: %v", err)
			http.Error(w, "proxy error after re-authentication: "+err.Error(), http.StatusBadGateway)
		}
	}
}
