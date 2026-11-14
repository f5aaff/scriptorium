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
	c.JSON(http.StatusOK, gin.H{"authenticated": true, "csrf_token": csrfToken, "session_token": sessionToken})
}

func (h *Handlers) Logout(c *gin.Context) {
	h.Manager.Logout(c.Request)
	h.Manager.ClearSessionCookies(c.Writer)
	c.JSON(http.StatusOK, gin.H{"authenticated": false})
}

func (h *Handlers) Me(c *gin.Context) {
	csrf, _ := c.Cookie(csrfCookie)
	c.JSON(http.StatusOK, gin.H{"authenticated": true, "csrf_token": csrf})
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
	if err := h.Manager.ChangePassword(request.CurrentPassword, request.NewPassword); err != nil {
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

func (h *Handlers) RequireSession() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !h.Manager.Authenticated(c.Request) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			return
		}
		method := c.Request.Method
		if method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions {
			if !h.Manager.ValidCSRF(c.Request) {
				c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "missing or invalid CSRF token"})
				return
			}
		}
		c.Next()
	}
}
