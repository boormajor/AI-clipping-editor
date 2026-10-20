package main

// ChatGPT plan sign-in uses OpenAI's documented open-source dynamic OAuth client.
// No ChatGPT session cookies, copied tokens, API keys, or undocumented endpoints are used.
import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const openAIAuthBase = "https://auth.openai.com"
const openAIResource = "https://api.openai.com/v1"
const openAIEndpoint = "https://api.openai.com/v1"

type chatProfile struct {
	HostID       string    `json:"host_id"`
	ClientID     string    `json:"client_id"`
	Email        string    `json:"email"`
	Subject      string    `json:"subject"`
	IDToken      string    `json:"id_token"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Scope        string    `json:"scope"`
	Expiry       time.Time `json:"expiry"`
}

type oauthPending struct {
	State    string
	Nonce    string
	Verifier string
	Redirect string
	ClientID string
	Started  time.Time
}

type chatAuth struct {
	mu         sync.Mutex
	profile    chatProfile
	current    *oauthPending
	errorText  string
	storage    string
	httpClient *http.Client
}

func randomURLBytes(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func makeHostID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("urn:uuid:%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
func newChatAuth() (*chatAuth, error) {
	d, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	directory := filepath.Join(d, "PodcastAIStudio")
	if err = os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}
	a := &chatAuth{storage: filepath.Join(directory, "chatgpt-profile.dat"), httpClient: &http.Client{Timeout: 45 * time.Second}}
	if b, e := os.ReadFile(a.storage); e == nil {
		if b, e = unprotectAccount(b); e != nil {
			return nil, fmt.Errorf("could not decrypt saved ChatGPT account: %w", e)
		}
		if e = json.Unmarshal(b, &a.profile); e != nil {
			return nil, fmt.Errorf("invalid saved ChatGPT account: %w", e)
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return nil, e
	}
	if a.profile.HostID == "" {
		a.profile.HostID, err = makeHostID()
		if err != nil {
			return nil, err
		}
		if err = a.saveLocked(); err != nil {
			return nil, err
		}
	}
	return a, nil
}
func (a *chatAuth) saveLocked() error {
	b, err := json.Marshal(a.profile)
	if err != nil {
		return err
	}
	b, err = protectAccount(b)
	if err != nil {
		return err
	}
	tmp := a.storage + ".tmp"
	if err = os.WriteFile(tmp, b, 0600); err != nil {
		return err
	}
	if err = os.Rename(tmp, a.storage); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
func (a *chatAuth) status() map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	return map[string]any{"connected": a.profile.AccessToken != "" && hasScope(a.profile.Scope, "chatgpt.tokens.use.direct"), "email": a.profile.Email, "pending": a.current != nil, "error": a.errorText, "clientRegistered": a.profile.ClientID != ""}
}
func hasScope(scopes, scope string) bool {
	for _, s := range strings.Fields(scopes) {
		if s == scope {
			return true
		}
	}
	return false
}
func (a *chatAuth) start() (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.current != nil && time.Since(a.current.Started) < 5*time.Minute {
		return "", errors.New("Sign-in is already in progress. Complete the open browser window first.")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	state, e := randomURLBytes(32)
	if e != nil {
		ln.Close()
		return "", e
	}
	nonce, e := randomURLBytes(32)
	if e != nil {
		ln.Close()
		return "", e
	}
	verifier, e := randomURLBytes(48)
	if e != nil {
		ln.Close()
		return "", e
	}
	port := ln.Addr().(*net.TCPAddr).Port
	pending := &oauthPending{State: state, Nonce: nonce, Verifier: verifier, Redirect: fmt.Sprintf("http://127.0.0.1:%d/auth/callback", port), ClientID: a.profile.ClientID, Started: time.Now()}
	a.current = pending
	a.errorText = ""
	vals := url.Values{"client_id": {a.profile.ClientID}, "response_type": {"code"}, "redirect_uri": {pending.Redirect}, "scope": {"openid profile email offline_access resource.invoke chatgpt.tokens.use.direct"}, "resource": {openAIResource}, "state": {state}, "nonce": {nonce}, "code_challenge_method": {"S256"}, "ext_agent_host_id": {a.profile.HostID}}
	if a.profile.ClientID == "" {
		vals.Set("client_id", "dynamic_agent_client")
		vals.Set("agent_name_hint", "Podcast AI Studio")
	} else {
		if a.profile.IDToken != "" {
			vals.Set("id_token_hint", a.profile.IDToken)
		}
		if a.profile.Email != "" {
			vals.Set("login_hint", a.profile.Email)
		}
	}
	digest := sha256.Sum256([]byte(verifier))
	vals.Set("code_challenge", base64.RawURLEncoding.EncodeToString(digest[:]))
	authorization := openAIAuthBase + "/api/accounts/authorize?" + vals.Encode()
	mux := http.NewServeMux()
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 8 * time.Second}
	mux.HandleFunc("/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		// Complete exactly one callback for the currently active attempt.
		a.mu.Lock()
		valid := a.current == pending && subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("state")), []byte(pending.State)) == 1
		if valid {
			a.current = nil
		}
		a.mu.Unlock()
		if !valid {
			http.Error(w, "Invalid or expired sign-in callback", http.StatusBadRequest)
			return
		}
		defer server.Close()
		if e := r.URL.Query().Get("error"); e != "" {
			a.setError("ChatGPT authorization declined: " + e)
			http.Error(w, "Authorization declined. Return to Podcast AI Studio.", 400)
			return
		}
		code := r.URL.Query().Get("code")
		clientID := r.URL.Query().Get("client_id")
		if code == "" {
			a.setError("ChatGPT callback did not include an authorization code")
			http.Error(w, "No authorization code returned", 400)
			return
		}
		if pending.ClientID == "" {
			if !strings.HasPrefix(clientID, "oaiapp_") {
				a.setError("OpenAI did not issue a dynamic client ID")
				http.Error(w, "Registration incomplete", 400)
				return
			}
		} else {
			if clientID != "" && clientID != pending.ClientID {
				a.setError("Unexpected client ID in callback")
				http.Error(w, "Unexpected client ID", 400)
				return
			}
			clientID = pending.ClientID
		}
		// Callback remains responsive while token exchange + signature verification happen.
		fmt.Fprint(w, "<html><body style='font:18px system-ui;padding:40px'><h2>Podcast AI Studio</h2><p>Sign-in received. Return to the app to check the connection status.</p></body></html>")
		go a.finish(pending, clientID, code)
	})
	go func() { _ = server.Serve(ln) }()
	go func() {
		time.Sleep(5 * time.Minute)
		a.mu.Lock()
		expired := a.current == pending
		if expired {
			a.current = nil
			a.errorText = "Sign-in expired. Please try again."
		}
		a.mu.Unlock()
		_ = server.Close()
	}()
	return authorization, nil
}
func (a *chatAuth) setError(s string) { a.mu.Lock(); a.errorText = s; a.mu.Unlock() }

type oauthToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	IDToken      string `json:"id_token"`
	Scope        string `json:"scope"`
	ExpiresIn    int    `json:"expires_in"`
}

func (a *chatAuth) exchange(ctx context.Context, values url.Values) (oauthToken, error) {
	var out oauthToken
	req, e := http.NewRequestWithContext(ctx, "POST", openAIAuthBase+"/api/accounts/oauth/token", strings.NewReader(values.Encode()))
	if e != nil {
		return out, e
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, e := a.httpClient.Do(req)
	if e != nil {
		return out, e
	}
	defer resp.Body.Close()
	data, e := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if e != nil {
		return out, e
	}
	if resp.StatusCode >= 300 {
		return out, fmt.Errorf("ChatGPT token exchange returned HTTP %d: %s", resp.StatusCode, string(data[:min(len(data), 300)]))
	}
	if e = json.Unmarshal(data, &out); e != nil {
		return out, e
	}
	if out.AccessToken == "" {
		return out, errors.New("token exchange returned no access token")
	}
	return out, nil
}
func (a *chatAuth) finish(p *oauthPending, clientID, code string) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	t, e := a.exchange(ctx, url.Values{"grant_type": {"authorization_code"}, "client_id": {clientID}, "code": {code}, "code_verifier": {p.Verifier}, "redirect_uri": {p.Redirect}, "resource": {openAIResource}})
	if e != nil {
		a.setError(e.Error())
		return
	}
	subject, email, e := a.verifyIDToken(ctx, t.IDToken, clientID, p.Nonce)
	if e != nil {
		a.setError("ID token validation failed: " + e.Error())
		return
	}
	if !hasScope(t.Scope, "chatgpt.tokens.use.direct") {
		a.setError("ChatGPT plan usage permission was not granted; please authorize it in the browser.")
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.profile.Subject != "" && a.profile.ClientID == clientID && a.profile.Subject != subject {
		a.errorText = "The connected ChatGPT account was changed unexpectedly"
		return
	}
	a.profile.ClientID = clientID
	a.profile.Subject = subject
	a.profile.Email = email
	a.profile.IDToken = t.IDToken
	a.profile.AccessToken = t.AccessToken
	a.profile.RefreshToken = t.RefreshToken
	a.profile.Scope = t.Scope
	a.profile.Expiry = time.Now().Add(time.Duration(t.ExpiresIn) * time.Second)
	if e = a.saveLocked(); e != nil {
		a.errorText = "Could not save the authenticated profile: " + e.Error()
	} else {
		a.errorText = ""
	}
}
func (a *chatAuth) accessToken(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.profile.AccessToken == "" || !hasScope(a.profile.Scope, "chatgpt.tokens.use.direct") {
		return "", errors.New("Connect your ChatGPT account and grant plan usage first")
	}
	if time.Now().Add(2 * time.Minute).Before(a.profile.Expiry) {
		return a.profile.AccessToken, nil
	}
	if a.profile.RefreshToken == "" {
		return "", errors.New("ChatGPT session has expired; please sign in again")
	}
	t, e := a.exchange(ctx, url.Values{"grant_type": {"refresh_token"}, "client_id": {a.profile.ClientID}, "refresh_token": {a.profile.RefreshToken}, "resource": {openAIResource}})
	if e != nil {
		return "", fmt.Errorf("could not refresh ChatGPT session: %w", e)
	}
	a.profile.AccessToken = t.AccessToken
	if t.RefreshToken != "" {
		a.profile.RefreshToken = t.RefreshToken
	}
	if t.Scope != "" {
		a.profile.Scope = t.Scope
	}
	if t.IDToken != "" {
		a.profile.IDToken = t.IDToken
	}
	a.profile.Expiry = time.Now().Add(time.Duration(t.ExpiresIn) * time.Second)
	if e = a.saveLocked(); e != nil {
		return "", e
	}
	return a.profile.AccessToken, nil
}
func (a *chatAuth) models(ctx context.Context) ([]map[string]string, error) {
	token, e := a.accessToken(ctx)
	if e != nil {
		return nil, e
	}
	req, e := http.NewRequestWithContext(ctx, "GET", openAIEndpoint+"/models", nil)
	if e != nil {
		return nil, e
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, e := a.httpClient.Do(req)
	if e != nil {
		return nil, e
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("model discovery HTTP %d: %s", resp.StatusCode, string(body[:min(250, len(body))]))
	}
	var data struct {
		Models []struct {
			Slug       string `json:"slug"`
			Name       string `json:"display_name"`
			Visibility string `json:"visibility"`
		} `json:"models"`
	}
	if e = json.Unmarshal(body, &data); e != nil {
		return nil, e
	}
	var models []map[string]string
	for _, m := range data.Models {
		if m.Slug != "" && m.Visibility == "list" {
			models = append(models, map[string]string{"slug": m.Slug, "name": m.Name})
		}
	}
	return models, nil
}
func (a *chatAuth) disconnect() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	host := a.profile.HostID
	a.profile = chatProfile{HostID: host}
	a.current = nil
	a.errorText = ""
	return a.saveLocked()
}
func (a *chatAuth) verifyIDToken(ctx context.Context, raw, clientID, nonce string) (string, string, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return "", "", errors.New("malformed JWT")
	}
	hd, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil {
		return "", "", e
	}
	var header struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if e = json.Unmarshal(hd, &header); e != nil {
		return "", "", e
	}
	if header.Alg != "RS256" || header.Kid == "" {
		return "", "", errors.New("unsupported JWT signing algorithm or missing key ID")
	}
	req, e := http.NewRequestWithContext(ctx, "GET", openAIAuthBase+"/.well-known/jwks.json", nil)
	if e != nil {
		return "", "", e
	}
	resp, e := a.httpClient.Do(req)
	if e != nil {
		return "", "", e
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("key lookup returned HTTP %d", resp.StatusCode)
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	var jwks struct {
		Keys []struct {
			Kid string `json:"kid"`
			Kty string `json:"kty"`
			N   string `json:"n"`
			E   string `json:"e"`
			Use string `json:"use"`
		} `json:"keys"`
	}
	if e = json.Unmarshal(data, &jwks); e != nil {
		return "", "", e
	}
	var key *rsa.PublicKey
	for _, j := range jwks.Keys {
		if j.Kid != header.Kid || j.Kty != "RSA" {
			continue
		}
		nb, e := base64.RawURLEncoding.DecodeString(j.N)
		if e != nil {
			return "", "", e
		}
		eb, e := base64.RawURLEncoding.DecodeString(j.E)
		if e != nil {
			return "", "", e
		}
		n := 0
		for _, v := range eb {
			n = n<<8 | int(v)
		}
		if n < 3 || n%2 == 0 {
			return "", "", errors.New("invalid JWK exponent")
		}
		key = &rsa.PublicKey{N: new(big.Int).SetBytes(nb), E: n}
		break
	}
	if key == nil {
		return "", "", errors.New("OpenAI signing key not found")
	}
	signature, e := base64.RawURLEncoding.DecodeString(parts[2])
	if e != nil {
		return "", "", e
	}
	hash := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if e = rsa.VerifyPKCS1v15(key, crypto.SHA256, hash[:], signature); e != nil {
		return "", "", fmt.Errorf("invalid token signature: %w", e)
	}
	pb, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil {
		return "", "", e
	}
	var claims struct {
		Issuer   string          `json:"iss"`
		Audience json.RawMessage `json:"aud"`
		Exp      int64           `json:"exp"`
		Issued   int64           `json:"iat"`
		Nonce    string          `json:"nonce"`
		Subject  string          `json:"sub"`
		Email    string          `json:"email"`
	}
	if e = json.Unmarshal(pb, &claims); e != nil {
		return "", "", e
	}
	validAud := false
	var aud string
	if json.Unmarshal(claims.Audience, &aud) == nil {
		validAud = aud == clientID
	} else {
		var arr []string
		if json.Unmarshal(claims.Audience, &arr) == nil {
			for _, s := range arr {
				if s == clientID {
					validAud = true
				}
			}
		}
	}
	now := time.Now().Unix()
	if claims.Issuer != openAIAuthBase || !validAud || claims.Exp <= now || claims.Issued > now+120 || claims.Nonce != nonce || claims.Subject == "" {
		return "", "", errors.New("token claims failed issuer, audience, expiry, nonce or subject checks")
	}
	return claims.Subject, claims.Email, nil
}
