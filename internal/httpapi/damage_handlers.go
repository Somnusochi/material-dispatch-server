package httpapi

import (
	"github.com/Somnusochi/material-dispatch-server/internal/damage"
	"github.com/gin-gonic/gin"
)

func (a *application) registerDamageRoutes(api *gin.RouterGroup) {
	api.GET("/damage-reports", a.list(a.damage.List))
	api.GET("/damage-reports/:id", func(c *gin.Context) {
		id, ok := a.id(c)
		if !ok {
			return
		}
		data, err := a.damage.Get(c.Request.Context(), id)
		a.respond(c, data, err, 200)
	})
	api.POST("/damage-reports", func(c *gin.Context) {
		var in damage.CreateInput
		if !a.bind(c, &in) {
			return
		}
		data, err := a.damage.Create(c.Request.Context(), actor(c), in)
		a.respond(c, data, err, 201)
	})
	for _, action := range []string{"confirm", "reject", "repair", "scrap"} {
		api.POST("/damage-reports/:id/"+action, roles("admin", "dispatcher"), func(c *gin.Context) {
			id, ok := a.id(c)
			if !ok {
				return
			}
			var in struct {
				Remark string `json:"remark"`
			}
			if !a.bind(c, &in) {
				return
			}
			data, err := a.damage.Transition(c.Request.Context(), actor(c), id, action, in.Remark)
			a.respond(c, data, err, 200)
		})
	}
}
