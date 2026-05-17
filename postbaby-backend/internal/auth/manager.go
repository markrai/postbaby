package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"postbaby-backend/internal/store"
)

var (
	ErrUnauthorized       = errors.New("unauthorized")
	ErrInvalidCredentials = errors.New("invalid credentials")
)

const (
	defaultCookieName      = "postbaby_session"
	defaultSessionTTL      = 30 * 24 * time.Hour
	defaultSessionTouchTTL = 5 * time.Minute
)

type Options struct {
	CookieName   string
	CookieSecure bool
	SessionTTL   time.Duration
	SessionTouch time.Duration
}

type Manager struct {
	store           store.IdentityStore
	cookieName      string
	cookieSecure    bool
	sessionTTL      time.Duration
	sessionTouchTTL time.Duration
}

func NewManager(docStore store.IdentityStore, options Options) *Manager {
	cookieName := strings.TrimSpace(options.CookieName)
	if cookieName == "" {
		cookieName = defaultCookieName
	}

	sessionTTL := options.SessionTTL
	if sessionTTL <= 0 {
		sessionTTL = defaultSessionTTL
	}

	sessionTouchTTL := options.SessionTouch
	if sessionTouchTTL <= 0 {
		sessionTouchTTL = defaultSessionTouchTTL
	}

	return &Manager{
		store:           docStore,
		cookieName:      cookieName,
		cookieSecure:    options.CookieSecure,
		sessionTTL:      sessionTTL,
		sessionTouchTTL: sessionTouchTTL,
	}
}

func (m *Manager) SetupRequired(ctx context.Context) (bool, error) {
	usersExist, err := m.store.UsersExist(ctx)
	if err != nil {
		return false, err
	}

	return !usersExist, nil
}

func (m *Manager) CreateInitialUser(ctx context.Context, username, password string) (store.User, error) {
	passwordHash, err := HashPassword(password)
	if err != nil {
		return store.User{}, err
	}

	ownerKey, err := m.store.BootstrapOwnerKey(ctx)
	if err != nil {
		return store.User{}, err
	}
	if ownerKey == "" {
		ownerKey, err = randomOwnerKey()
		if err != nil {
			return store.User{}, err
		}
	}

	return m.store.CreateInitialUser(ctx, username, passwordHash, ownerKey)
}

func (m *Manager) CreateUser(ctx context.Context, username, password string) (store.User, error) {
	passwordHash, err := HashPassword(password)
	if err != nil {
		return store.User{}, err
	}

	ownerKey, err := randomOwnerKey()
	if err != nil {
		return store.User{}, err
	}

	return m.store.CreateUser(ctx, username, passwordHash, ownerKey)
}

func (m *Manager) Login(ctx context.Context, w http.ResponseWriter, username, password string) (store.User, error) {
	user, err := m.store.GetUserByUsername(ctx, username)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			return store.User{}, ErrInvalidCredentials
		}
		return store.User{}, err
	}

	ok, err := VerifyPassword(password, user.PasswordHash)
	if err != nil {
		return store.User{}, err
	}
	if !ok {
		return store.User{}, ErrInvalidCredentials
	}

	if err := m.CreateSession(ctx, w, user.ID); err != nil {
		return store.User{}, err
	}

	return user, nil
}

func (m *Manager) CreateSession(ctx context.Context, w http.ResponseWriter, userID int64) error {
	rawToken, err := randomSessionToken()
	if err != nil {
		return err
	}

	expiresAt := time.Now().UTC().Add(m.sessionTTL)
	if _, err := m.store.CreateSession(ctx, userID, hashToken(rawToken), expiresAt); err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     m.cookieName,
		Value:    rawToken,
		Path:     "/",
		MaxAge:   int(time.Until(expiresAt).Seconds()),
		Expires:  expiresAt,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   m.cookieSecure,
	})

	return nil
}

func (m *Manager) AuthenticateRequest(ctx context.Context, w http.ResponseWriter, r *http.Request) (*store.User, error) {
	rawToken, err := m.sessionTokenFromRequest(r)
	if err != nil {
		m.ClearSessionCookie(w)
		return nil, ErrUnauthorized
	}

	sessionUser, err := m.store.GetSessionUserByTokenHash(ctx, hashToken(rawToken))
	if err != nil {
		if errors.Is(err, store.ErrSessionNotFound) {
			m.ClearSessionCookie(w)
			return nil, ErrUnauthorized
		}
		return nil, err
	}

	now := time.Now().UTC()
	if !sessionUser.Session.ExpiresAt.After(now) {
		_ = m.store.DeleteSessionByTokenHash(ctx, sessionUser.Session.TokenHash)
		m.ClearSessionCookie(w)
		return nil, ErrUnauthorized
	}

	if now.Sub(sessionUser.Session.LastSeenAt) >= m.sessionTouchTTL {
		if err := m.store.TouchSession(ctx, sessionUser.Session.ID, now); err != nil {
			return nil, err
		}
	}

	return &sessionUser.User, nil
}

func (m *Manager) EndSession(ctx context.Context, w http.ResponseWriter, r *http.Request) error {
	rawToken, err := m.sessionTokenFromRequest(r)
	if err == nil {
		if deleteErr := m.store.DeleteSessionByTokenHash(ctx, hashToken(rawToken)); deleteErr != nil {
			return deleteErr
		}
	}

	m.ClearSessionCookie(w)
	return nil
}

func (m *Manager) ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     m.cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(0, 0).UTC(),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   m.cookieSecure,
	})
}

func (m *Manager) sessionTokenFromRequest(r *http.Request) (string, error) {
	cookie, err := r.Cookie(m.cookieName)
	if err != nil {
		return "", ErrUnauthorized
	}
	value := strings.TrimSpace(cookie.Value)
	if value == "" {
		return "", ErrUnauthorized
	}

	return value, nil
}

func randomOwnerKey() (string, error) {
	randomBytes := make([]byte, 16)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("read owner key bytes: %w", err)
	}

	return "user_" + hex.EncodeToString(randomBytes), nil
}

func randomSessionToken() (string, error) {
	randomBytes := make([]byte, 32)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("read session token bytes: %w", err)
	}

	return base64.RawURLEncoding.EncodeToString(randomBytes), nil
}

func hashToken(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
