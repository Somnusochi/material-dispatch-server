package httpapi

import (
	"io"
	"net/http"

	"github.com/Somnusochi/material-dispatch-server/internal/platform/business"
	"github.com/Somnusochi/material-dispatch-server/internal/upload"
	"github.com/gin-gonic/gin"
)

func (a *application) registerUploadRoutes(api *gin.RouterGroup) {
	api.POST("/uploads", a.uploadImage)
	api.GET("/uploads/:id", func(c *gin.Context) {
		path, mime, err := a.upload.Read(c.Request.Context(), c.Param("id"))
		if err != nil {
			a.respond(c, nil, err, 0)
			return
		}
		c.Header("Content-Type", mime)
		c.Header("Cache-Control", "private, max-age=300")
		c.Header("Content-Disposition", "inline")
		c.File(path)
	})
}

func (a *application) uploadImage(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, upload.MaxBytes+(64<<10))
	file, header, err := c.Request.FormFile("file")
	if c.Request.MultipartForm != nil {
		defer c.Request.MultipartForm.RemoveAll()
	}
	if err != nil {
		a.respond(c, nil, business.Invalid("请上传5MB以内的图片"), 0)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, upload.MaxBytes+1))
	if err != nil {
		a.respond(c, nil, business.Invalid("读取图片失败"), 0)
		return
	}
	result, err := a.upload.Save(c.Request.Context(), actor(c), header.Filename, data)
	a.respond(c, result, err, 201)
}
