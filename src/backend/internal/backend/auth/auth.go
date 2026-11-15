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
	ErrUserExists         = errors.New("username already exists")
	ErrUserNotFound       = errors.New("user not found")
	ErrLastAdmin          = errors.New("cannot disable or demote the last administrator")
	ErrForbidden          = errors.New("permission denied")
)

type Permissions struct {
	Upload bool `json:"upload"`
	Edit   bool `json:"edit"`
	Delete bool `json:"delete"`
}

type User struct {
	Username     string      `json:"username"`
	Admin        bool        `json:"admin"`
	Disabled     bool        `json:"disabled"`
	Permissions  Permissions `json:"permissions"`
	PasswordHash string      `json:"password_hash,omitempty"`
}

type credentialFile struct {
	// Legacy fields allow a single-account auth file to be migrated in place.
	Username     string          `json:"username,omitempty"`
	PasswordHash string          `json:"password_hash,omitempty"`
	Users        map[string]User `json:"users,omitempty"`
}

type session struct {
	username  string
	csrfToken string
	expiresAt time.Time
}

type Manager struct {
	mu           sync.Mutex
	path         string
	users        map[string]User
	cookieSecure bool
	sessions     map[string]session
	failures     map[string][]time.Time
}

func NewManager(path, bootstrapUsername, bootstrapPassword string, cookieSecure bool) (*Manager, error) {
	m := &Manager{path: path, cookieSecure: cookieSecure, users: make(map[string]User), sessions: make(map[string]session), failures: make(map[string][]time.Time)}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, errors.New("authentication file must be a regular, non-symlink file")
		}
		if err := os.Chmod(path, 0600); err != nil {
			return nil, fmt.Errorf("secure existing authentication file: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("inspect authentication file: %w", err)
	}

	data, err := os.ReadFile(path)
	if err == nil {
		var stored credentialFile
		if err := json.Unmarshal(data, &stored); err != nil {
			return nil, fmt.Errorf("decode authentication file: %w", err)
		}
		if len(stored.Users) > 0 {
			for key, user := range stored.Users {
				if key == "" || user.Username == "" || user.PasswordHash == "" {
					return nil, errors.New("authentication file contains an incomplete user")
				}
				m.users[key] = user
			}
		} else if stored.Username != "" && stored.PasswordHash != "" {
			// Preserve the existing owner account and grant it administrator access.
			m.users[stored.Username] = User{Username: stored.Username, Admin: true, Permissions: Permissions{Upload: true, Edit: true, Delete: true}, PasswordHash: stored.PasswordHash}
			if err := m.persistLocked(); err != nil {
				return nil, fmt.Errorf("migrate authentication file: %w", err)
			}
		} else {
			return nil, errors.New("authentication file is incomplete")
		}
		admins := 0
		for _, user := range m.users {
			if user.Admin && !user.Disabled {
				admins++
			}
		}
		if admins == 0 {
			return nil, errors.New("authentication file has no enabled administrator")
		}
		return m, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read authentication file: %w", err)
	}

	bootstrapUsername = strings.TrimSpace(bootstrapUsername)
	if bootstrapUsername == "" || bootstrapPassword == "" {
		return nil, errors.New("no owner account exists; set AUTH_USERNAME and AUTH_PASSWORD for first startup")
	}
	if err := validateCredentials(bootstrapUsername, bootstrapPassword); err != nil {
		return nil, err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(bootstrapPassword), passwordCost)
	if err != nil {
		return nil, fmt.Errorf("hash bootstrap password: %w", err)
	}
	m.users[bootstrapUsername] = User{Username: bootstrapUsername, Admin: true, Permissions: Permissions{Upload: true, Edit: true, Delete: true}, PasswordHash: string(hash)}
	if err := m.persistLocked(); err != nil {
		return nil, err
	}
	return m, nil
}

func validateCredentials(username, password string) error {
	if username == "" || len(username) > maxUsername {
		return fmt.Errorf("username must be between 1 and %d bytes", maxUsername)
	}
	if utf8.RuneCountInString(password) < minPassword {
		return ErrWeakPassword
	}
	if len(password) > 72 {
		return errors.New("password must not exceed 72 bytes")
	}
	return nil
}

func (m *Manager) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0700); err != nil {
		return fmt.Errorf("create authentication directory: %w", err)
	}
	data, err := json.MarshalIndent(credentialFile{Users: m.users}, "", "  ")
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
	user, ok := m.users[username]
	passwordErr := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if !ok || user.Disabled || passwordErr != nil {
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
	m.sessions[sessionToken] = session{username: username, csrfToken: csrfToken, expiresAt: now.Add(sessionTTL)}
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

func (m *Manager) SessionUser(r *http.Request) (User, bool) {
	token := requestSessionToken(r)
	if token == "" {
		return User{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[token]
	if !ok || time.Now().After(s.expiresAt) {
		delete(m.sessions, token)
		return User{}, false
	}
	u, ok := m.users[s.username]
	if !ok || u.Disabled {
		delete(m.sessions, token)
		return User{}, false
	}
	return publicUser(u), true
}
func publicUser(u User) User                          { u.PasswordHash = ""; return u }
func (m *Manager) Authenticated(r *http.Request) bool { _, ok := m.SessionUser(r); return ok }
func (m *Manager) ValidCSRF(r *http.Request) bool {
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
	return subtle.ConstantTimeCompare([]byte(csrfCookieValue.Value), []byte(s.csrfToken)) == 1 && subtle.ConstantTimeCompare([]byte(header), []byte(s.csrfToken)) == 1
}
func (m *Manager) Logout(r *http.Request) {
	if token := requestSessionToken(r); token != "" {
		m.mu.Lock()
		delete(m.sessions, token)
		m.mu.Unlock()
	}
}

func (m *Manager) ChangePassword(usernameOrOldPassword string, passwords ...string) error {
	var username, oldPassword, newPassword string
	if len(passwords) == 1 {
		// Retain the former two-argument API for existing callers/tests.
		oldPassword, newPassword = usernameOrOldPassword, passwords[0]
		m.mu.Lock()
		for name, user := range m.users {
			if user.Admin {
				username = name
				break
			}
		}
		m.mu.Unlock()
	} else if len(passwords) == 2 {
		username, oldPassword, newPassword = usernameOrOldPassword, passwords[0], passwords[1]
	} else {
		return errors.New("invalid password change request")
	}
	if err := validateCredentials(username, newPassword); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[username]
	if !ok || bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(oldPassword)) != nil {
		return ErrInvalidCredentials
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), passwordCost)
	if err != nil {
		return fmt.Errorf("hash new password: %w", err)
	}
	previous := u.PasswordHash
	u.PasswordHash = string(hash)
	m.users[username] = u
	if err := m.persistLocked(); err != nil {
		u.PasswordHash = previous
		m.users[username] = u
		return err
	}
	for token, s := range m.sessions {
		if s.username == username {
			delete(m.sessions, token)
		}
	}
	return nil
}

func (m *Manager) ListUsers() []User {
	m.mu.Lock()
	defer m.mu.Unlock()
	users := make([]User, 0, len(m.users))
	for _, u := range m.users {
		users = append(users, publicUser(u))
	}
	return users
}
func (m *Manager) CreateUser(username, password string, permissions Permissions) error {
	username = strings.TrimSpace(username)
	if err := validateCredentials(username, password); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), passwordCost)
	if err != nil {
		return fmt.Errorf("hash user password: %w", err)
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.users[username]; exists {
		return ErrUserExists
	}
	m.users[username] = User{Username: username, Permissions: permissions, PasswordHash: string(hash)}
	if err := m.persistLocked(); err != nil {
		delete(m.users, username)
		return err
	}
	return nil
}
func (m *Manager) UpdateUser(username string, permissions Permissions, disabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[username]
	if !ok {
		return ErrUserNotFound
	}
	if u.Admin {
		if disabled || permissions != (Permissions{Upload: true, Edit: true, Delete: true}) {
			return ErrForbidden
		}
		return nil
	}
	previous := u
	u.Permissions = permissions
	u.Disabled = disabled
	m.users[username] = u
	if err := m.persistLocked(); err != nil {
		m.users[username] = previous
		return err
	}
	if disabled {
		for token, s := range m.sessions {
			if s.username == username {
				delete(m.sessions, token)
			}
		}
	}
	return nil
}
func (m *Manager) DeleteUser(username string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[username]
	if !ok {
		return ErrUserNotFound
	}
	if u.Admin {
		admins := 0
		for _, candidate := range m.users {
			if candidate.Admin && !candidate.Disabled {
				admins++
			}
		}
		if admins <= 1 {
			return ErrLastAdmin
		}
	}
	delete(m.users, username)
	if err := m.persistLocked(); err != nil {
		m.users[username] = u
		return err
	}
	for token, s := range m.sessions {
		if s.username == username {
			delete(m.sessions, token)
		}
	}
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
