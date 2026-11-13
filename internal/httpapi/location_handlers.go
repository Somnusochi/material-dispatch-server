package httpapi

import (
	"github.com/Somnusochi/material-dispatch-server/internal/catalog"
	"github.com/gin-gonic/gin"
)

func (a *application) registerLocationRoutes(api *gin.RouterGroup) {
	api.GET("/locations", a.list(a.catalog.Locations))
	routes := a.options.Routes
	api.GET("/locations/:id/destinations", roles("admin", "dispatcher"), func(c *gin.Context) {
		page, err := parsePage(c)
		if err != nil {
			a.respond(c, nil, err, 0)
			return
		}
		data, err := routes.Destinations(c.Request.Context(), c.Param("id"), page)
		a.respond(c, data, err, 200)
	})
	saveLocation := func(c *gin.Context) {
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
		var in catalog.LocationInput
		if !a.bind(c, &in) {
			return
		}
		data, err := a.catalog.SaveLocation(c.Request.Context(), actor(c), id, in)
		a.respond(c, data, err, status)
	}
	api.POST("/locations", roles("admin", "dispatcher"), saveLocation)
	api.PUT("/locations/:id", roles("admin", "dispatcher"), saveLocation)
}
