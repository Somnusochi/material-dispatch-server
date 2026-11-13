package httpapi

import (
	"github.com/Somnusochi/material-dispatch-server/internal/catalog"
	"github.com/gin-gonic/gin"
)

func (a *application) registerSpecRoutes(api *gin.RouterGroup) {
	api.GET("/material-specs", a.list(a.catalog.Specs))
	saveSpec := func(c *gin.Context) {
		id := int64(0)
		status := 201
		if c.Param("id") != "" {
			var ok bool
			id, ok = a.id(c)
			if !ok {
				return
			}
			status = 200
		}
		var in catalog.SpecInput
		if !a.bind(c, &in) {
			return
		}
		data, err := a.catalog.SaveSpec(c.Request.Context(), actor(c), id, in)
		a.respond(c, data, err, status)
	}
	api.POST("/material-specs", roles("admin", "dispatcher"), saveSpec)
	api.PUT("/material-specs/:id", roles("admin", "dispatcher"), saveSpec)
}
