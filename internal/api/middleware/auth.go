// Package middleware holds cross-cutting Gin handlers (auth, logging, rate
// limiting) applied across route groups.
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/MohamedShetewi/order-processing-system/internal/auth"
	"github.com/MohamedShetewi/order-processing-system/internal/models"
)

// Context keys under which the authenticated principal is stored. They are
// unexported; handlers read them through the accessor helpers below.
const (
	ctxKeyUserID = "auth.userID"
	ctxKeyRole   = "auth.role"
)

// Authenticate verifies the Bearer token on the request and, on success, stores
// the caller's user ID and role in the Gin context. Requests without a valid
// token are rejected with 401 before reaching the handler.
func Authenticate(tokens auth.TokenManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if header == "" {
			abortUnauthorized(c, "authorization header required")
			return
		}

		// Expect exactly "Bearer <token>".
		scheme, token, found := strings.Cut(header, " ")
		if !found || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
			abortUnauthorized(c, "invalid authorization header")
			return
		}

		claims, err := tokens.Parse(strings.TrimSpace(token))
		if err != nil {
			abortUnauthorized(c, "invalid or expired token")
			return
		}

		c.Set(ctxKeyUserID, claims.UserID)
		c.Set(ctxKeyRole, claims.Role)
		c.Next()
	}
}

// RequireAdmin guards a route so only an authenticated admin may proceed. It is
// a convenience wrapper over RequireRole so callers (e.g. the router) don't need
// to import the models package. It must run after Authenticate.
func RequireAdmin() gin.HandlerFunc {
	return RequireRole(models.UserRoleAdmin)
}

// RequireRole guards a route so only an authenticated caller with the given role
// may proceed. It must run after Authenticate.
func RequireRole(role models.UserRole) gin.HandlerFunc {
	return func(c *gin.Context) {
		got, ok := Role(c)
		if !ok || got != string(role) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}
		c.Next()
	}
}

// UserID returns the authenticated user's ID, or false if the request was not
// authenticated.
func UserID(c *gin.Context) (int, bool) {
	v, ok := c.Get(ctxKeyUserID)
	if !ok {
		return 0, false
	}
	id, ok := v.(int)
	return id, ok
}

// Role returns the authenticated user's role, or false if absent.
func Role(c *gin.Context) (string, bool) {
	v, ok := c.Get(ctxKeyRole)
	if !ok {
		return "", false
	}
	role, ok := v.(string)
	return role, ok
}

func abortUnauthorized(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": msg})
}
