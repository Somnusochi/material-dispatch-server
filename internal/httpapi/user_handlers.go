package httpapi

import (
	"github.com/Somnusochi/material-dispatch-server/internal/auth"
	"github.com/gin-gonic/gin"
)

func (a *application) registerUserRoutes(api *gin.RouterGroup) {
	api.GET("/contacts", a.list(a.auth.Contacts))
	api.GET("/users", roles("admin"), a.list(a.auth.List))
	api.POST("/users", roles("admin"), func(c *gin.Context) {
		var in auth.UserInput
		if !a.bind(c, &in) {
			return
		}
		data, err := a.auth.CreateUser(c.Request.Context(), actor(c), in)
		a.respond(c, data, err, 201)
	})
	api.PUT("/users/:id", roles("admin"), func(c *gin.Context) {
		id, ok := a.id(c)
		if !ok {
			return
		}
		var in auth.UserInput
		if !a.bind(c, &in) {
			return
		}
		data, err := a.auth.UpdateUser(c.Request.Context(), actor(c), id, in)
		a.respond(c, data, err, 200)
	})
}
