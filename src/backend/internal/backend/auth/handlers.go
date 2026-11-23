package auth

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

type Handlers struct{ Manager *Manager }

func (h *Handlers) Login(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8*1024)
	var request struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || strings.TrimSpace(request.Username) == "" || len(request.Username) > maxUsername || request.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password are required"})
		return
	}
	sessionToken, csrfToken, err := h.Manager.Login(request.Username, request.Password, c.ClientIP())
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
			return
		}
		if errors.Is(err, ErrRateLimited) {
			c.Header("Retry-After", "900")
			c.JSON(http.StatusTooManyRequests, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create session"})
		return
	}
	h.Manager.SetSessionCookies(c.Writer, sessionToken, csrfToken)
	user, _ := h.Manager.SessionUser(tokenRequest(sessionToken))
	c.JSON(http.StatusOK, gin.H{"authenticated": true, "csrf_token": csrfToken, "session_token": sessionToken, "user": user})
}

// tokenRequest builds a request carrying a freshly-issued bearer token so the
// login response can include the exact account's effective permissions.
func tokenRequest(token string) *http.Request {
	r, _ := http.NewRequest(http.MethodGet, "/auth/me", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	return r
}

func (h *Handlers) Logout(c *gin.Context) {
	h.Manager.Logout(c.Request)
	h.Manager.ClearSessionCookies(c.Writer)
	c.JSON(http.StatusOK, gin.H{"authenticated": false})
}
func (h *Handlers) Me(c *gin.Context) {
	user, _ := h.Manager.SessionUser(c.Request)
	csrf, _ := c.Cookie(csrfCookie)
	c.JSON(http.StatusOK, gin.H{"authenticated": true, "csrf_token": csrf, "user": user})
}

func (h *Handlers) ChangePassword(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8*1024)
	var request struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid password change request"})
		return
	}
	user, ok := h.Manager.SessionUser(c.Request)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	if err := h.Manager.ChangePassword(user.Username, request.CurrentPassword, request.NewPassword); err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "current password is incorrect"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.Manager.ClearSessionCookies(c.Writer)
	c.JSON(http.StatusOK, gin.H{"message": "password changed; sign in again"})
}

func (h *Handlers) ListUsers(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"users": h.Manager.ListUsers()})
}
func (h *Handlers) CreateUser(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8*1024)
	var request struct {
		Username    string      `json:"username"`
		Password    string      `json:"password"`
		Permissions Permissions `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user request"})
		return
	}
	if err := h.Manager.CreateUser(request.Username, request.Password, request.Permissions); err != nil {
		if errors.Is(err, ErrUserExists) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"message": "user created"})
}
func (h *Handlers) UpdateUser(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8*1024)
	var request struct {
		Permissions Permissions `json:"permissions"`
		Disabled    bool        `json:"disabled"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user update"})
		return
	}
	if err := h.Manager.UpdateUser(c.Param("username"), request.Permissions, request.Disabled); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrUserNotFound) {
			status = http.StatusNotFound
		} else if errors.Is(err, ErrForbidden) {
			status = http.StatusForbidden
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "user updated"})
}
func (h *Handlers) DeleteUser(c *gin.Context) {
	if err := h.Manager.DeleteUser(c.Param("username")); err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, ErrUserNotFound) {
			status = http.StatusNotFound
		} else if errors.Is(err, ErrForbidden) {
			status = http.StatusForbidden
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "user deleted"})
}

const sessionUserKey = "scriptorium.session.user"

// CurrentUser returns the account authenticated by RequireSession, never a
// principal supplied in a request body or query parameter.
func CurrentUser(c *gin.Context) (User, bool) {
	value, exists := c.Get(sessionUserKey)
	user, ok := value.(User)
	return user, exists && ok && user.ID != ""
}

func (h *Handlers) RequireSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		user, authenticated := h.Manager.SessionUser(c.Request)
		if !authenticated {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		c.Set(sessionUserKey, user)
		method := c.Request.Method
		if method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions && !h.Manager.ValidCSRF(c.Request) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "missing or invalid CSRF token"})
			return
		}
		c.Next()
	}
}
func (h *Handlers) RequireAdmin() gin.HandlerFunc {
	return h.require(func(u User) bool { return u.Admin })
}
func (h *Handlers) RequirePermission(permission string) gin.HandlerFunc {
	return h.require(func(u User) bool {
		if u.Admin {
			return true
		}
		switch permission {
		case "upload":
			return u.Permissions.Upload
		case "edit":
			return u.Permissions.Edit
		case "delete":
			return u.Permissions.Delete
		default:
			return false
		}
	})
}
func (h *Handlers) require(allowed func(User) bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, ok := h.Manager.SessionUser(c.Request)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		if !allowed(user) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "permission denied"})
			return
		}
		c.Next()
	}
}
