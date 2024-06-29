package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

var userDB map[string]string

func initUserDB() error {
	var err error
	userDB = make(map[string]string)
	bytes, err := os.ReadFile("./data/users.json")
	if err != nil {
		return err
	}
	if err = json.Unmarshal(bytes, &userDB); err != nil {
		return err
	}
	return nil
}

func saveUserDB() error {
	var err error
	jsonBody, err := json.Marshal(userDB)
	if err != nil {
		return err
	}
	err = os.WriteFile("./data/users.json", jsonBody, 0666)
	if err != nil {
		return err
	}
	return nil
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
	if err := initUserDB(); err != nil {
		log.Fatal("failed to load user.json: ", err)
	}

	r := setupRouter()

	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	// Initializing the server in a goroutine so that
	// it won't block the graceful shutdown handling below
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server with
	// a timeout of 5 seconds.
	quit := make(chan os.Signal, 1)
	// kill (no param) default send syscall.SIGTERM
	// kill -2 is syscall.SIGINT
	// kill -9 is syscall.SIGKILL but can't be catch, so don't need add it
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// The context is used to inform the server it has 5 seconds to finish
	// the request it is currently handling
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown: ", err)
	}

	log.Println("save user.json")
	if err := saveUserDB(); err != nil {
		log.Fatal("failed in saveUserDB: ", err)
	}

	log.Println("Server exiting")
}
