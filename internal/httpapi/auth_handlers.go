package httpapi

import (
	"errors"
	"math"
	"net/http"
	"time"

	"github.com/Somnusochi/material-dispatch-server/internal/auth"
	"github.com/Somnusochi/material-dispatch-server/internal/platform/business"
	"github.com/gin-gonic/gin"
)

const sessionCookie = "material_dispatch_session"
const refreshCookie = "material_dispatch_refresh"

func (a *application) registerPublicAuth(api *gin.RouterGroup, origins map[string]bool) {
	api.POST("/auth/login", loginLimiter(), a.login)
	api.POST("/auth/refresh", func(c *gin.Context) {
		if !origins[c.GetHeader("Origin")] || c.GetHeader("X-Refresh-Request") != "1" {
			failure(c, http.StatusForbidden, "ORIGIN_NOT_ALLOWED", "当前访问地址未获允许，请使用配置的系统地址")
			return
		}
		token, _ := c.Cookie(refreshCookie)
		session, tokens, err := a.auth.Refresh(c.Request.Context(), token, actor(c))
		if err != nil {
			var problem *business.Error
			if errors.As(err, &problem) && problem.Status == 401 {
				a.clearCookies(c)
			}
			a.respond(c, nil, err, 0)
			return
		}
		a.tokenCookies(c, session, tokens)
		success(c, session)
	})
}

func (a *application) registerAccountRoutes(api *gin.RouterGroup) {
	api.GET("/auth/me", func(c *gin.Context) {
		success(c, c.MustGet("auth_session"))
	})
	api.POST("/auth/logout", func(c *gin.Context) {
		err := a.auth.Logout(c.Request.Context(), c.GetString("session"), actor(c))
		if err == nil {
			a.clearCookies(c)
		}
		a.respond(c, gin.H{"ok": true}, err, 200)
	})
	api.POST("/auth/password", func(c *gin.Context) {
		var in struct {
			Current string `json:"current_password"`
			New     string `json:"new_password"`
		}
		if !a.bind(c, &in) {
			return
		}
		err := a.auth.ChangePassword(c.Request.Context(), actor(c), in.Current, in.New)
		if err == nil {
			a.clearCookies(c)
		}
		a.respond(c, gin.H{"ok": true}, err, 200)
	})
}
func (a *application) cookie(c *gin.Context, name, token, path string, age int) {
	http.SetCookie(c.Writer, &http.Cookie{Name: name, Value: token, Path: path, MaxAge: age, HttpOnly: true, Secure: a.options.SecureCookies, SameSite: http.SameSiteStrictMode})
}
func (a *application) clearCookies(c *gin.Context) {
	a.cookie(c, sessionCookie, "", "/api/v1", -1)
	a.cookie(c, refreshCookie, "", "/api/v1/auth", -1)
}
func (a *application) tokenCookies(c *gin.Context, session auth.Session, tokens auth.Tokens) {
	a.cookie(c, sessionCookie, tokens.Access, "/api/v1", int(math.Ceil(time.Until(session.AccessExpiresAt).Seconds())))
	a.cookie(c, refreshCookie, tokens.Refresh, "/api/v1/auth", int(math.Ceil(time.Until(session.RefreshExpiresAt).Seconds())))
}
func (a *application) login(c *gin.Context) {
	var in struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !a.bind(c, &in) {
		return
	}
	session, tokens, err := a.auth.Login(c.Request.Context(), in.Username, in.Password, actor(c))
	if err != nil {
		a.respond(c, nil, err, 0)
		return
	}
	a.tokenCookies(c, session, tokens)
	success(c, session)
}
