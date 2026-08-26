package identity

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/fastygo/backend/internal/domain/authz"
	frameworkauth "github.com/fastygo/framework/pkg/auth"
)

const SessionCookieName = "headless_session"

const tokenVersion = 1

type Claims struct {
	Version      int                `json:"v"`
	Issuer       string             `json:"iss"`
	Subject      string             `json:"sub"`
	Capabilities []authz.Capability `json:"capabilities"`
	IssuedAt     int64              `json:"iat"`
	ExpiresAt    int64              `json:"exp"`
}

type TokenManager struct {
	secret string
	issuer string
	now    func() time.Time
}

func NewTokenManager(secret, issuer string) (*TokenManager, error) {
	if len(secret) < 32 {
		return nil, errors.New("token secret must contain at least 32 bytes")
	}
	issuer = strings.TrimSpace(issuer)
	if issuer == "" {
		return nil, errors.New("token issuer is required")
	}
	return &TokenManager{secret: secret, issuer: issuer, now: time.Now}, nil
}

func (manager *TokenManager) Issue(principal authz.Principal, ttl time.Duration) (string, error) {
	if strings.TrimSpace(principal.ID) == "" || principal.Anonymous {
		return "", errors.New("authenticated principal is required")
	}
	if ttl <= 0 {
		return "", errors.New("token TTL must be positive")
	}
	capabilities := make([]authz.Capability, 0, len(principal.Capabilities))
	for capability := range principal.Capabilities {
		if !capability.Valid() {
			return "", errors.New("principal contains an invalid capability")
		}
		capabilities = append(capabilities, capability)
	}
	slices.Sort(capabilities)
	now := manager.now().UTC()
	return frameworkauth.SignedEncode(Claims{
		Version: tokenVersion, Issuer: manager.issuer, Subject: principal.ID,
		Capabilities: capabilities, IssuedAt: now.Unix(), ExpiresAt: now.Add(ttl).Unix(),
	}, manager.secret)
}

func (manager *TokenManager) CSRF(token string) string {
	mac := hmac.New(sha256.New, []byte(manager.secret))
	_, _ = mac.Write([]byte("csrf|"))
	_, _ = mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}

func (manager *TokenManager) ValidateCookieCSRF(request *http.Request) error {
	if strings.TrimSpace(request.Header.Get("Authorization")) != "" {
		return nil
	}
	cookie, err := request.Cookie(SessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return nil
	}
	expected := manager.CSRF(cookie.Value)
	provided := strings.TrimSpace(request.Header.Get("X-CSRF-Token"))
	if provided == "" || !hmac.Equal([]byte(provided), []byte(expected)) {
		return errors.New("csrf token is invalid")
	}
	return nil
}

func (manager *TokenManager) Resolve(request *http.Request) (authz.Principal, error) {
	token, err := requestToken(request)
	if err != nil {
		return authz.Principal{}, err
	}
	if token == "" {
		return authz.Anonymous(), nil
	}
	var claims Claims
	if err := frameworkauth.SignedDecode(token, manager.secret, &claims); err != nil {
		return authz.Principal{}, errors.New("bearer token is invalid")
	}
	now := manager.now().UTC().Unix()
	switch {
	case claims.Version != tokenVersion:
		return authz.Principal{}, errors.New("bearer token version is unsupported")
	case claims.Issuer != manager.issuer:
		return authz.Principal{}, errors.New("bearer token issuer is invalid")
	case strings.TrimSpace(claims.Subject) == "":
		return authz.Principal{}, errors.New("bearer token subject is missing")
	case claims.ExpiresAt <= now:
		return authz.Principal{}, errors.New("bearer token has expired")
	case claims.IssuedAt > now+30:
		return authz.Principal{}, errors.New("bearer token was issued in the future")
	}
	for _, capability := range claims.Capabilities {
		if !capability.Valid() {
			return authz.Principal{}, errors.New("bearer token capability is invalid")
		}
	}
	return authz.NewPrincipal(claims.Subject, claims.Capabilities...), nil
}

func requestToken(request *http.Request) (string, error) {
	authorization := strings.TrimSpace(request.Header.Get("Authorization"))
	if authorization != "" {
		scheme, token, found := strings.Cut(authorization, " ")
		if !found || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
			return "", errors.New("authorization must use Bearer authentication")
		}
		return strings.TrimSpace(token), nil
	}
	cookie, err := request.Cookie(SessionCookieName)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return "", nil
	}
	return strings.TrimSpace(cookie.Value), nil
}
