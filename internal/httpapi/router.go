package httpapi

import (
	"context"
	"crypto/rand"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
)

type Pinger interface{ Ping(context.Context) error }

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
type response struct {
	Data      any       `json:"data,omitempty"`
	Error     *apiError `json:"error,omitempty"`
	RequestID string    `json:"request_id"`
}

func NewRouter(db Pinger, logger *slog.Logger) *gin.Engine {
	router := gin.New()
	router.HandleMethodNotAllowed = true
	_ = router.SetTrustedProxies(nil)
	router.Use(requestContext(logger), recoverPanic(logger))
	router.GET("/healthz", func(c *gin.Context) { success(c, gin.H{"status": "ok"}) })
	ready := func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		if err := db.Ping(ctx); err != nil {
			failure(c, http.StatusServiceUnavailable, "DATABASE_UNAVAILABLE", "数据库暂不可用")
			return
		}
		success(c, gin.H{"service": "material-dispatch-server", "status": "ready", "database": "up"})
	}
	router.GET("/readyz", ready)
	router.Group("/api/v1").GET("/system/status", ready)
	router.NoRoute(func(c *gin.Context) { failure(c, http.StatusNotFound, "NOT_FOUND", "接口不存在") })
	router.NoMethod(func(c *gin.Context) {
		failure(c, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "请求方法不支持")
	})
	return router
}
func success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, response{Data: data, RequestID: c.GetString("request_id")})
}
func failure(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, response{Error: &apiError{Code: code, Message: message}, RequestID: c.GetString("request_id")})
}
func requestContext(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if !validRequestID.MatchString(id) {
			id = rand.Text()
		}
		c.Set("request_id", id)
		c.Header("X-Request-ID", id)
		started := time.Now()
		c.Next()
		logger.Info("http request", "request_id", id, "method", c.Request.Method, "path", c.Request.URL.Path, "status", c.Writer.Status(), "duration_ms", time.Since(started).Milliseconds())
	}
}
func recoverPanic(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if recover() != nil {
				logger.Error("request panic", "request_id", c.GetString("request_id"))
				if !c.Writer.Written() {
					failure(c, http.StatusInternalServerError, "INTERNAL_ERROR", "服务暂时不可用")
				} else {
					c.Abort()
				}
			}
		}()
		c.Next()
	}
}
