package httpapi

import (
	"context"
	"strconv"
	"strings"

	"github.com/Somnusochi/material-dispatch-server/internal/platform/business"
	"github.com/gin-gonic/gin"
)

func (a *application) list(fn func(context.Context, business.Page) (business.List, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		page, err := parsePage(c)
		if err != nil {
			a.respond(c, nil, err, 0)
			return
		}
		data, err := fn(c.Request.Context(), page)
		a.respond(c, data, err, 200)
	}
}
func parsePage(c *gin.Context) (business.Page, error) {
	p := business.Page{Keyword: strings.TrimSpace(c.Query("keyword")), Status: c.Query("status"), LocationID: c.Query("location_id"), SpecID: c.Query("spec_id"), Module: c.Query("module"), Action: c.Query("action")}
	var err error
	p.Page, err = strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || p.Page < 1 || p.Page > 100000 {
		return p, business.Invalid("分页页码无效")
	}
	p.Size, err = strconv.Atoi(c.DefaultQuery("page_size", "20"))
	if err != nil || p.Size < 1 || p.Size > 100 {
		return p, business.Invalid("每页数量需为1至100")
	}
	for _, value := range []string{p.Keyword, p.Status, p.Module, p.Action} {
		if len(value) > 256 {
			return p, business.Invalid("筛选条件过长")
		}
	}
	for _, id := range []string{p.LocationID, p.SpecID} {
		if id != "" {
			if _, err = business.ID(id); err != nil {
				return p, err
			}
		}
	}
	return p, nil
}
