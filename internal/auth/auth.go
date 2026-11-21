// Package auth implements Google and Okta sign-in (OpenID Connect, authorization code
// flow with PKCE, state and nonce) and cookie sessions backed by the database.
package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/andriantam/ace-mate/internal/config"
	"github.com/andriantam/ace-mate/internal/db"
)

type Provider struct {
	ID       string
	Name     string
	oauth    *oauth2.Config
	verifier *oidc.IDTokenVerifier
}

type Auth struct {
	DB        *db.DB
	Providers []*Provider
	secure    bool
	dev       bool
	birthday  bool // also ask Google for the birthday (AUTH_GOOGLE_BIRTHDAY)
	http      *http.Client
}

const (
	sessionCookie = "am_session"
	flowCookie    = "am_oauth"
)

func cookieName(base string, secure bool) string {
	if secure {
		return "__Host-" + base
	}
	return base
}

// New discovers the configured providers. A provider that can't be reached is skipped with a warning.
func New(ctx context.Context, c *config.Config, d *db.DB) *Auth {
	a := &Auth{DB: d, secure: c.Secure(), dev: c.DevLogin, birthday: c.GoogleBirthday, http: &http.Client{Timeout: 15 * time.Second}}
	add := func(id, name, issuer string, o config.OAuth) {
		// openid, email and profile give the name, given and family name, picture, email,
		// email-verified flag and locale. Nothing else is requested by default.
		scopes := []string{oidc.ScopeOpenID, "email", "profile"}
		if id == "google" && c.GoogleBirthday {
			scopes = append(scopes, birthdayScope)
		}
		if !o.Enabled() {
			return
		}
		dctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		p, err := oidc.NewProvider(dctx, issuer)
		if err != nil {
			slog.Warn("sign-in provider unavailable", "provider", id, "err", err)
			return
		}
		a.Providers = append(a.Providers, &Provider{
			ID: id, Name: name,
			oauth: &oauth2.Config{
				ClientID: o.ID, ClientSecret: o.Secret, Endpoint: p.Endpoint(),
				RedirectURL: c.BaseURL + "/api/auth/callback/" + id,
				Scopes:      scopes,
			},
			verifier: p.Verifier(&oidc.Config{ClientID: o.ID}),
		})
	}
	add("google", "Google", "https://accounts.google.com", c.Google)
	if c.Okta.Enabled() && c.Okta.Issuer != "" {
		add("okta", "Okta", c.Okta.Issuer, c.Okta)
	}
	return a
}

func (a *Auth) provider(id string) *Provider {
	for _, p := range a.Providers {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func randToken(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

type flowState struct {
	Provider, State, Nonce, Verifier string
}

// Login starts the sign-in redirect for the provider in the path.
func (a *Auth) Login(w http.ResponseWriter, r *http.Request) {
	p := a.provider(r.PathValue("provider"))
	if p == nil {
		http.NotFound(w, r)
		return
	}
	fs := flowState{Provider: p.ID, State: randToken(24), Nonce: randToken(24), Verifier: oauth2.GenerateVerifier()}
	raw, _ := json.Marshal(fs)
	http.SetCookie(w, &http.Cookie{
		Name: cookieName(flowCookie, a.secure), Value: base64.RawURLEncoding.EncodeToString(raw),
		Path: "/", MaxAge: 600, HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, p.oauth.AuthCodeURL(fs.State, oauth2.S256ChallengeOption(fs.Verifier), oidc.Nonce(fs.Nonce)), http.StatusFound)
}

type claims struct {
	Subject       string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified *bool  `json:"email_verified"`
	Name          string `json:"name"`
	GivenName     string `json:"given_name"`
	FamilyName    string `json:"family_name"`
	Picture       string `json:"picture"`
	Locale        string `json:"locale"`
	Nonce         string `json:"nonce"`
}

// Callback completes sign-in: verifies state, exchanges the code, validates the ID token,
// then creates a session. Provider tokens are used once and never stored.
func (a *Auth) Callback(w http.ResponseWriter, r *http.Request) {
	fail := func(msg string, err error) {
		slog.Warn("sign-in failed", "reason", msg, "err", err)
		http.Redirect(w, r, "/signin?error="+url.QueryEscape(msg), http.StatusFound)
	}
	p := a.provider(r.PathValue("provider"))
	if p == nil {
		http.NotFound(w, r)
		return
	}
	ck, err := r.Cookie(cookieName(flowCookie, a.secure))
	if err != nil {
		fail("Sign-in expired. Please try again.", err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName(flowCookie, a.secure), Path: "/", MaxAge: -1, HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode})
	raw, err := base64.RawURLEncoding.DecodeString(ck.Value)
	var fs flowState
	if err != nil || json.Unmarshal(raw, &fs) != nil || fs.Provider != p.ID {
		fail("Sign-in expired. Please try again.", errors.New("bad flow cookie"))
		return
	}
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		fail("Sign-in was cancelled.", errors.New(e))
		return
	}
	if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(fs.State)) != 1 {
		fail("Sign-in failed. Please try again.", errors.New("state mismatch"))
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	tok, err := p.oauth.Exchange(ctx, q.Get("code"), oauth2.VerifierOption(fs.Verifier))
	if err != nil {
		fail("Sign-in failed. Please try again.", err)
		return
	}
	rawID, _ := tok.Extra("id_token").(string)
	idt, err := p.verifier.Verify(ctx, rawID)
	if err != nil {
		fail("Sign-in failed. Please try again.", err)
		return
	}
	var cl claims
	if err := idt.Claims(&cl); err != nil || cl.Subject == "" {
		fail("Sign-in failed. Please try again.", err)
		return
	}
	if subtle.ConstantTimeCompare([]byte(cl.Nonce), []byte(fs.Nonce)) != 1 {
		fail("Sign-in failed. Please try again.", errors.New("nonce mismatch"))
		return
	}
	if cl.EmailVerified != nil && !*cl.EmailVerified {
		fail("Your email address isn't verified with that provider.", errors.New("email not verified"))
		return
	}
	image := ""
	if u, err := url.Parse(cl.Picture); err == nil && u.Scheme == "https" {
		image = cl.Picture
	}
	in := db.Identity{
		Provider: p.ID, Subject: cl.Subject, Email: cl.Email, Name: clip(cl.Name), Image: image,
		GivenName: clip(cl.GivenName), FamilyName: clip(cl.FamilyName), Locale: clip(cl.Locale),
		EmailVerified: cl.EmailVerified != nil && *cl.EmailVerified,
	}
	if p.ID == "google" && a.birthday {
		// The access token is used for this one call and never stored.
		if b, err := googleBirthday(ctx, p.oauth.TokenSource(ctx, tok), a.http); err != nil {
			slog.Warn("birthday lookup failed", "err", err)
		} else {
			in.Birthday = b
		}
	}
	a.startSession(w, r, in)
}

func clip(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func (a *Auth) startSession(w http.ResponseWriter, r *http.Request, in db.Identity) {
	uid, err := a.DB.UpsertIdentity(r.Context(), in)
	if err != nil {
		slog.Error("upsert user", "err", err)
		http.Redirect(w, r, "/signin?error="+url.QueryEscape("Something went wrong. Please try again."), http.StatusFound)
		return
	}
	tok, exp, err := a.DB.CreateSession(r.Context(), uid)
	if err != nil {
		slog.Error("create session", "err", err)
		http.Redirect(w, r, "/signin?error="+url.QueryEscape("Something went wrong. Please try again."), http.StatusFound)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName(sessionCookie, a.secure), Value: tok, Path: "/", Expires: exp,
		HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

// DevLogin signs in as a throwaway local user. Only registered when DEV_LOGIN is on
// (config refuses that outside http://localhost).
func (a *Auth) DevLogin(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.URL.Query().Get("user"))
	if name == "" || len(name) > 40 || strings.ContainsAny(name, "<>&\"'") {
		name = "dev"
	}
	a.startSession(w, r, db.Identity{Provider: "dev", Subject: name, Email: name + "@localhost", Name: name, GivenName: name, EmailVerified: true})
}

func (a *Auth) DevEnabled() bool { return a.dev }

// ── sessions ──

func (a *Auth) token(r *http.Request) string {
	if c, err := r.Cookie(cookieName(sessionCookie, a.secure)); err == nil {
		return c.Value
	}
	return ""
}

// CurrentUser returns the signed-in user, or nil.
func (a *Auth) CurrentUser(r *http.Request) *db.User {
	t := a.token(r)
	if t == "" {
		return nil
	}
	u, err := a.DB.UserBySession(r.Context(), t)
	if err != nil {
		return nil
	}
	return u
}

func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	if t := a.token(r); t != "" {
		_ = a.DB.DeleteSession(r.Context(), t)
	}
	a.ClearCookie(w)
}

func (a *Auth) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: cookieName(sessionCookie, a.secure), Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode})
}
