package httpapi

import (
	"github.com/Somnusochi/material-dispatch-server/internal/inventory"
	"github.com/gin-gonic/gin"
)

func (a *application) registerInventoryRoutes(api *gin.RouterGroup) {
	api.GET("/inventory", a.list(a.inventory.List))
	api.POST("/inventory/adjust", roles("admin"), func(c *gin.Context) {
		var in inventory.AdjustInput
		if !a.bind(c, &in) {
			return
		}
		data, err := a.inventory.Adjust(c.Request.Context(), actor(c), in)
		a.respond(c, data, err, 200)
	})
}
