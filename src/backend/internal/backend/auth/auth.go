package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	sessionCookie = "scriptorium_session"
	csrfCookie    = "scriptorium_csrf"
	sessionTTL    = 12 * time.Hour
	minPassword   = 12
	maxUsername   = 128
	passwordCost  = 12
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrWeakPassword       = fmt.Errorf("password must contain at least %d characters", minPassword)
	ErrRateLimited        = errors.New("too many login attempts; try again later")
)

type credentialFile struct {
	Username     string `json:"username"`
	PasswordHash string `json:"password_hash"`
}

type session struct {
	csrfToken string
	expiresAt time.Time
}

type Manager struct {
	mu           sync.Mutex
	path         string
	username     string
	passwordHash []byte
	cookieSecure bool
	sessions     map[string]session
	failures     map[string][]time.Time
}

// NewManager loads an existing credential file or bootstraps the first owner.
// It intentionally refuses to start without credentials on a fresh install.
func NewManager(path, bootstrapUsername, bootstrapPassword string, cookieSecure bool) (*Manager, error) {
	m := &Manager{
		path:         path,
		cookieSecure: cookieSecure,
		sessions:     make(map[string]session),
		failures:     make(map[string][]time.Time),
	}

	if info, statErr := os.Lstat(path); statErr == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, errors.New("authentication file must be a regular, non-symlink file")
		}
		if err := os.Chmod(path, 0600); err != nil {
			return nil, fmt.Errorf("secure existing authentication file: %w", err)
		}
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect authentication file: %w", statErr)
	}
	data, err := os.ReadFile(path)
	if err == nil {
		var stored credentialFile
		if err := json.Unmarshal(data, &stored); err != nil {
			return nil, fmt.Errorf("decode authentication file: %w", err)
		}
		if strings.TrimSpace(stored.Username) == "" || stored.PasswordHash == "" {
			return nil, errors.New("authentication file is incomplete")
		}
		m.username = stored.Username
		m.passwordHash = []byte(stored.PasswordHash)
		return m, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read authentication file: %w", err)
	}

	bootstrapUsername = strings.TrimSpace(bootstrapUsername)
	if bootstrapUsername == "" || bootstrapPassword == "" {
		return nil, errors.New("no owner account exists; set AUTH_USERNAME and AUTH_PASSWORD for first startup")
	}
	if len(bootstrapUsername) > maxUsername {
		return nil, fmt.Errorf("username must not exceed %d characters", maxUsername)
	}
	if utf8.RuneCountInString(bootstrapPassword) < minPassword {
		return nil, ErrWeakPassword
	}
	if len(bootstrapPassword) > 72 {
		return nil, errors.New("password must not exceed 72 bytes")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(bootstrapPassword), passwordCost)
	if err != nil {
		return nil, fmt.Errorf("hash bootstrap password: %w", err)
	}
	m.username = bootstrapUsername
	m.passwordHash = hash
	if err := m.persistLocked(); err != nil {
		return nil, err
	}
	return m, nil
}

func (m *Manager) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0700); err != nil {
		return fmt.Errorf("create authentication directory: %w", err)
	}
	data, err := json.MarshalIndent(credentialFile{Username: m.username, PasswordHash: string(m.passwordHash)}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode authentication file: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(m.path), ".scriptorium-auth-*")
	if err != nil {
		return fmt.Errorf("create authentication temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("secure authentication file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write authentication file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync authentication file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close authentication file: %w", err)
	}
	if err := os.Rename(tmpName, m.path); err != nil {
		return fmt.Errorf("install authentication file: %w", err)
	}
	return nil
}

func (m *Manager) Login(username, password, remoteAddr string) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	for token, existing := range m.sessions {
		if now.After(existing.expiresAt) {
			delete(m.sessions, token)
		}
	}
	recent := m.failures[remoteAddr][:0]
	for _, failedAt := range m.failures[remoteAddr] {
		if now.Sub(failedAt) < 15*time.Minute {
			recent = append(recent, failedAt)
		}
	}
	m.failures[remoteAddr] = recent
	if len(recent) >= 5 {
		return "", "", ErrRateLimited
	}
	usernameMatch := len(username) <= maxUsername && subtle.ConstantTimeCompare([]byte(username), []byte(m.username)) == 1
	passwordErr := bcrypt.CompareHashAndPassword(m.passwordHash, []byte(password))
	if !usernameMatch || passwordErr != nil {
		m.failures[remoteAddr] = append(m.failures[remoteAddr], now)
		return "", "", ErrInvalidCredentials
	}
	delete(m.failures, remoteAddr)

	sessionToken, err := randomToken(32)
	if err != nil {
		return "", "", err
	}
	csrfToken, err := randomToken(32)
	if err != nil {
		return "", "", err
	}
	m.sessions[sessionToken] = session{csrfToken: csrfToken, expiresAt: now.Add(sessionTTL)}
	return sessionToken, csrfToken, nil
}

func randomToken(size int) (string, error) {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate security token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func requestSessionToken(r *http.Request) string {
	if header := r.Header.Get("Authorization"); strings.HasPrefix(header, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	}
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (m *Manager) Authenticated(r *http.Request) bool {
	token := requestSessionToken(r)
	if token == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[token]
	if !ok || time.Now().After(s.expiresAt) {
		delete(m.sessions, token)
		return false
	}
	return true
}

func (m *Manager) ValidCSRF(r *http.Request) bool {
	// Explicit bearer tokens are not ambient browser credentials, so CSRF does
	// not apply. This also supports the Wails desktop webview, whose origin is
	// intentionally different from the local HTTP API.
	if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		return m.Authenticated(r)
	}
	token := requestSessionToken(r)
	csrfCookieValue, err := r.Cookie(csrfCookie)
	if err != nil {
		return false
	}
	header := r.Header.Get("X-CSRF-Token")
	if header == "" {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[token]
	if !ok || time.Now().After(s.expiresAt) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(csrfCookieValue.Value), []byte(s.csrfToken)) == 1 &&
		subtle.ConstantTimeCompare([]byte(header), []byte(s.csrfToken)) == 1
}

func (m *Manager) Logout(r *http.Request) {
	if token := requestSessionToken(r); token != "" {
		m.mu.Lock()
		delete(m.sessions, token)
		m.mu.Unlock()
	}
}

func (m *Manager) ChangePassword(oldPassword, newPassword string) error {
	if utf8.RuneCountInString(newPassword) < minPassword {
		return ErrWeakPassword
	}
	if len(newPassword) > 72 {
		return errors.New("password must not exceed 72 bytes")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := bcrypt.CompareHashAndPassword(m.passwordHash, []byte(oldPassword)); err != nil {
		return ErrInvalidCredentials
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), passwordCost)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}
	previous := m.passwordHash
	m.passwordHash = hash
	if err := m.persistLocked(); err != nil {
		m.passwordHash = previous
		return err
	}
	clear(m.sessions)
	return nil
}

func (m *Manager) SetSessionCookies(w http.ResponseWriter, sessionToken, csrfToken string) {
	secure := m.cookieSecure
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: sessionToken, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode, MaxAge: int(sessionTTL.Seconds())})
	http.SetCookie(w, &http.Cookie{Name: csrfCookie, Value: csrfToken, Path: "/", HttpOnly: false, Secure: secure, SameSite: http.SameSiteStrictMode, MaxAge: int(sessionTTL.Seconds())})
}

func (m *Manager) ClearSessionCookies(w http.ResponseWriter) {
	for _, name := range []string{sessionCookie, csrfCookie} {
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", HttpOnly: name == sessionCookie, Secure: m.cookieSecure, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	}
}
