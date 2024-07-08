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
	"github.com/go-crypt/crypt"
	"github.com/go-crypt/crypt/algorithm"
	"github.com/go-crypt/crypt/algorithm/argon2"
	"github.com/gorilla/securecookie"
)

const (
	USER_FILE = "./data/users.json"
)

// Hash keys should be at least 32 bytes long
var hashKey = []byte("very-secret")

// Block keys should be 16 bytes (AES-128) or 32 bytes (AES-256) long.
// Shorter keys may weaken the encryption used.
// var blockKey = []byte("very-secret")
var s = securecookie.New(hashKey, nil)

var userDB map[string]string

func initUserDB() error {
	var err error
	userDB = make(map[string]string)
	bytes, err := os.ReadFile(USER_FILE)
	if os.IsNotExist(err) {
		return nil
	}
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
	err = os.WriteFile(USER_FILE, jsonBody, 0666)
	if err != nil {
		return err
	}
	return nil
}

func generateHashedPassword(password string) (string, error) {
	var (
		hasher *argon2.Hasher
		err    error
		digest algorithm.Digest
	)

	if hasher, err = argon2.New(
		argon2.WithProfileRFC9106LowMemory(),
	); err != nil {
		return "", err
	}
	if digest, err = hasher.Hash(password); err != nil {
		return "", err
	}

	return digest.Encode(), nil
}

func checkHashedPassword(password string, hashedPassword string) (bool, error) {
	return crypt.CheckPassword(password, hashedPassword)
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
		var err error
		username := c.PostForm("username")
		password := c.PostForm("password")
		hashedPassword, err := generateHashedPassword(password)
		if err != nil {
			c.String(http.StatusInternalServerError, "failed on generateHashedPassword")
			return
		}
		userDB[username] = hashedPassword
		c.String(http.StatusOK, username)
	})

	r.GET("/login", func(c *gin.Context) {
		c.HTML(http.StatusOK, "login.tmpl", gin.H{})
	})
	r.POST("/login", func(c *gin.Context) {
		formUsername := c.PostForm("username")
		formPassword := c.PostForm("password")
		hashedPassword, ok := userDB[formUsername]
		if !ok {
			c.String(http.StatusForbidden, "not authorized")
			return
		}
		var err error
		checkResult, err := checkHashedPassword(formPassword, hashedPassword)
		if err != nil {
			c.String(http.StatusInternalServerError, "failed on checkHashedPassword")
			return
		}
		if !checkResult {
			c.String(http.StatusForbidden, "not authorized")
			return
		}

		var encoded string
		if encoded, err = s.Encode("cookie-name", formUsername); err != nil {
			c.String(http.StatusInternalServerError, "failed on encoding a cookie")
			return
		}

		c.SetCookie("cookie-name", encoded, 3600, "/", "" /* hostname */, false, true)
		c.String(http.StatusOK, "authorized")
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
