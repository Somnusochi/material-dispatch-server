package httpapi

import (
	"github.com/gin-gonic/gin"
)

func (a *application) registerMapRoutes(api *gin.RouterGroup) {
	api.GET("/map/locations", func(c *gin.Context) {
		data, err := a.catalog.Map(c.Request.Context(), c.Query("spec_id"))
		a.respond(c, data, err, 200)
	})
	api.GET("/map/locations/:id/materials", func(c *gin.Context) {
		page, err := parsePage(c)
		if err != nil {
			a.respond(c, nil, err, 0)
			return
		}
		data, err := a.catalog.MapMaterials(c.Request.Context(), c.Param("id"), page)
		a.respond(c, data, err, 200)
	})
	api.GET("/map/summary", func(c *gin.Context) {
		data, err := a.catalog.MapSummary(c.Request.Context(), c.Query("spec_id"), c.Query("keyword"), c.Query("location_id"))
		a.respond(c, data, err, 200)
	})
}
