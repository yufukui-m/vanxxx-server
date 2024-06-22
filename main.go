package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

var userDB map[string]string

func initUserDB() {
	userDB = make(map[string]string)
}

func setupRouter() *gin.Engine {
	r := gin.Default()
	r.LoadHTMLGlob("templates/*")

	// Ping test
	r.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	r.GET("/signup", func(c *gin.Context) {
		c.HTML(http.StatusOK, "signup.tmpl", gin.H{})
	})
	r.POST("/signup", func(c *gin.Context) {
		username := c.PostForm("username")
		password := c.PostForm("password")
		userDB[username] = password
		c.String(http.StatusOK, username)
	})

	return r
}

func main() {
	initUserDB()
	r := setupRouter()
	// Listen and Server in 0.0.0.0:8080
	r.Run(":8080")
}
