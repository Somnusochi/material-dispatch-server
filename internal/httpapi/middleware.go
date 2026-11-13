package httpapi

import (
	"context"
	"crypto/subtle"
	"net/http"
	"time"

	"github.com/Somnusochi/material-dispatch-server/internal/auth"
	"github.com/Somnusochi/material-dispatch-server/internal/platform/business"
	"github.com/gin-gonic/gin"
)

func requestPolicy(origins map[string]bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Cache-Control", "no-store")
		if c.Request.Method != "GET" && c.Request.Method != "HEAD" {
			origin := c.GetHeader("Origin")
			if origin != "" && !origins[origin] {
				failure(c, http.StatusForbidden, "ORIGIN_NOT_ALLOWED", "当前访问地址未获允许，请使用配置的系统地址")
				return
			}
		}
		c.Next()
	}
}
func actor(c *gin.Context) business.Actor {
	value, _ := c.Get("user")
	user, _ := value.(auth.User)
	return business.Actor{ID: user.ID, Username: user.Username, Role: user.Role, RequestID: c.GetString("request_id"), Method: c.Request.Method, Path: c.Request.URL.Path, IP: c.ClientIP()}
}
func (a *application) authenticate(c *gin.Context) {
	token, _ := c.Cookie(sessionCookie)
	session, err := a.auth.Authenticate(c.Request.Context(), token)
	if err != nil {
		a.respond(c, nil, err, 0)
		return
	}
	if c.Request.Method != "GET" && c.Request.Method != "HEAD" {
		if subtle.ConstantTimeCompare([]byte(c.GetHeader("X-CSRF-Token")), []byte(session.CSRF)) != 1 {
			a.respond(c, nil, business.Forbidden(), 0)
			return
		}
	}
	c.Set("user", session.User)
	c.Set("auth_session", session)
	c.Set("session", token)
	c.Next()
}
func roles(allowed ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user := c.MustGet("user").(auth.User)
		for _, role := range allowed {
			if user.Role == role {
				c.Next()
				return
			}
		}
		failure(c, 403, "FORBIDDEN", "没有此操作权限")
	}
}
