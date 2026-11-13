package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/Somnusochi/material-dispatch-server/internal/platform/business"
	"github.com/gin-gonic/gin"
)

func (a *application) bind(c *gin.Context, value any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		a.respond(c, nil, business.Invalid("请求数据格式不正确"), 0)
		return false
	}
	body = bytes.Trim(body, " \t\r\n")
	if len(body) == 0 || body[0] != '{' {
		a.respond(c, nil, business.Invalid("请求必须是 JSON 对象"), 0)
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		a.respond(c, nil, business.Invalid("请求数据格式不正确"), 0)
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		a.respond(c, nil, business.Invalid("请求只能包含一个 JSON 对象"), 0)
		return false
	}
	return true
}
func (a *application) id(c *gin.Context) (int64, bool) {
	id, err := business.ID(c.Param("id"))
	if err != nil {
		a.respond(c, nil, err, 0)
		return 0, false
	}
	return id, true
}
