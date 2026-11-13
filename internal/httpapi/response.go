package httpapi

import (
	"errors"

	"github.com/Somnusochi/material-dispatch-server/internal/platform/business"
	"github.com/gin-gonic/gin"
)

func (a *application) respond(c *gin.Context, data any, err error, status int) {
	if err == nil {
		c.JSON(status, response{Data: data, RequestID: c.GetString("request_id")})
		return
	}
	var problem *business.Error
	if errors.As(business.Normalize(err), &problem) {
		failure(c, problem.Status, problem.Code, problem.Message)
		return
	}
	a.logger.Error("business request failed", "request_id", c.GetString("request_id"), "path", c.Request.URL.Path)
	failure(c, 500, "INTERNAL_ERROR", "服务暂时不可用，请稍后重试")
}
