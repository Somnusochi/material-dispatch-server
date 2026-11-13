package httpapi

import (
	"github.com/Somnusochi/material-dispatch-server/internal/dispatch"
	"github.com/gin-gonic/gin"
)

func (a *application) registerDispatchRoutes(api *gin.RouterGroup) {
	api.GET("/dispatches", a.list(a.dispatch.List))
	api.GET("/dispatches/:id", func(c *gin.Context) {
		id, ok := a.id(c)
		if !ok {
			return
		}
		data, err := a.dispatch.Get(c.Request.Context(), id)
		a.respond(c, data, err, 200)
	})
	api.POST("/dispatches", roles("admin", "dispatcher"), func(c *gin.Context) {
		var in dispatch.CreateInput
		if !a.bind(c, &in) {
			return
		}
		data, err := a.dispatch.Create(c.Request.Context(), actor(c), in)
		a.respond(c, data, err, 201)
	})
	for _, action := range []string{"cancel", "depart", "receive"} {
		allowed := []string{"admin", "dispatcher"}
		if action == "receive" {
			allowed = append(allowed, "inspector")
		}
		api.POST("/dispatches/:id/"+action, roles(allowed...), func(c *gin.Context) {
			id, ok := a.id(c)
			if !ok {
				return
			}
			var in dispatch.ActionInput
			if !a.bind(c, &in) {
				return
			}
			data, err := a.dispatch.Transition(c.Request.Context(), actor(c), id, action, in)
			a.respond(c, data, err, 200)
		})
	}
}
