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

	r.GET("/", func(c *gin.Context) {
		username, _ := c.Cookie("username")

		c.HTML(http.StatusOK, "index.tmpl", gin.H{
			"username": username,
		})
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

	r.GET("/login", func(c *gin.Context) {
		c.HTML(http.StatusOK, "login.tmpl", gin.H{})
	})
	r.POST("/login", func(c *gin.Context) {
		formUsername := c.PostForm("username")
		formPassword := c.PostForm("password")
		password, ok := userDB[formUsername]
		if ok && password == formPassword {
			c.SetCookie("username", formUsername, 3600, "/", "" /* hostname */, false, true)
			c.String(http.StatusOK, "authorized")
		} else {
			c.String(http.StatusForbidden, "not authorized")
		}
	})

	r.GET("/logout", func(c *gin.Context) {
		_, err := c.Cookie("username")
		if err != nil {
			c.String(http.StatusForbidden, "not logged in")
		} else {
			c.HTML(http.StatusOK, "logout.tmpl", gin.H{})
		}
	})
	r.POST("/logout", func(c *gin.Context) {
		// note for Max-Age: https://blog.risouf.net/entry/2023-02-10-2023-02-10-golang-maxage-caution.html
		c.SetCookie("username", "", -1, "/", "" /* hostname */, false, true)
		c.String(http.StatusOK, "logged out")
	})

	return r
}

func main() {
	initUserDB()
	r := setupRouter()
	// Listen and Server in 0.0.0.0:8080
	r.Run(":8080")
}
