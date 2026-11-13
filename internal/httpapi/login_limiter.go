package httpapi

import (
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

func loginLimiter() gin.HandlerFunc {
	type entry struct {
		count int
		until time.Time
	}
	var mu sync.Mutex
	entries := map[string]entry{}
	return func(c *gin.Context) {
		mu.Lock()
		now := time.Now()
		for key, value := range entries {
			if now.After(value.until) {
				delete(entries, key)
			}
		}
		ip := c.ClientIP()
		value := entries[ip]
		if value.until.IsZero() {
			value.until = now.Add(time.Minute)
		}
		value.count++
		entries[ip] = value
		blocked := value.count > 20 || len(entries) > 4096
		mu.Unlock()
		if blocked {
			c.Header("Retry-After", "60")
			failure(c, 429, "RATE_LIMITED", "登录尝试过于频繁，请稍后重试")
			return
		}
		c.Next()
	}
}
