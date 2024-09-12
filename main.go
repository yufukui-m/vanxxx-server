package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-crypt/crypt"
	"github.com/go-crypt/crypt/algorithm"
	"github.com/go-crypt/crypt/algorithm/argon2"
	"github.com/gorilla/securecookie"
	"github.com/oklog/ulid/v2"

	"github.com/go-sql-driver/mysql"
)

const (
	USER_FILE           = "./data/users.json"
	SESSION_COOKIE_NAME = "session"
)

var s *securecookie.SecureCookie

func initSecureCookie() {
	// Hash keys should be at least 32 bytes long
	hashKey := []byte(os.Getenv("SESSION_HASH_KEY"))
	//  block key, the key should be 16, 24, or 32 bytes of random bits. set nil if encryption is not require
	blockKey, err := base64.StdEncoding.DecodeString(os.Getenv("SESSION_BLOCK_KEY"))
	if err != nil {
		panic(err)
	}

	s = securecookie.New(hashKey, blockKey)
}

func getSession(c *gin.Context) (string, error) {
	var err error
	var sessionCookie string
	if sessionCookie, err = c.Cookie(SESSION_COOKIE_NAME); err != nil {
		return "", err
	}
	var userId string
	if err = s.Decode(SESSION_COOKIE_NAME, sessionCookie, &userId); err != nil {
		return "", err
	}

	return userId, nil
}

func setSession(c *gin.Context, userId string) error {
	encoded, err := s.Encode(SESSION_COOKIE_NAME, userId)
	if err != nil {
		return err
	}

	c.SetCookie(SESSION_COOKIE_NAME, encoded, 3600, "/", "" /* hostname */, false, true)
	return nil
}

func expireSession(c *gin.Context) {
	// note for Max-Age: https://blog.risouf.net/entry/2023-02-10-2023-02-10-golang-maxage-caution.html
	c.SetCookie(SESSION_COOKIE_NAME, "", -1, "/", "" /* hostname */, false, true)
}

type UserAttr struct {
	UserId         string
	Nickname       string
	HashedPassword string
}

var userDB = []UserAttr{}

func getUserAttrFromNickname(nickname string) (UserAttr, error) {
	for _, v := range userDB {
		if v.Nickname == nickname {
			return v, nil
		}
	}
	return UserAttr{}, errors.New("not found")
}

func initUserDB() error {
	var err error
	userDB = make([]UserAttr, 0)
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
		userId, _ := getSession(c)

		c.HTML(http.StatusOK, "index.tmpl", gin.H{
			"username": userId,
		})
	})

	r.GET("/signup", func(c *gin.Context) {
		c.HTML(http.StatusOK, "signup.tmpl", gin.H{})
	})
	r.POST("/signup", func(c *gin.Context) {
		var err error
		nickname := c.PostForm("username")
		password := c.PostForm("password")

		// nickname から userAttr を引いてくる
		_, err = getUserAttrFromNickname(nickname)
		if err == nil {
			c.String(http.StatusForbidden, "user already exists")
		}

		hashedPassword, err := generateHashedPassword(password)
		if err != nil {
			c.String(http.StatusInternalServerError, "failed on generateHashedPassword")
			return
		}

		// signup の時にユーザid を生成
		userId := "u-" + ulid.Make().String()

		userDB = append(userDB, UserAttr{
			userId,
			nickname,
			hashedPassword,
		})
		c.String(http.StatusOK, nickname)
	})

	r.GET("/login", func(c *gin.Context) {
		c.HTML(http.StatusOK, "login.tmpl", gin.H{})
	})
	r.POST("/login", func(c *gin.Context) {
		formUsername := c.PostForm("username")
		formPassword := c.PostForm("password")

		var err error
		userAttr, err := getUserAttrFromNickname(formUsername)
		if err != nil {
			// ユーザがいない
			c.String(http.StatusForbidden, "not authorized")
		}

		checkResult, err := checkHashedPassword(formPassword, userAttr.HashedPassword)
		if err != nil {
			c.String(http.StatusInternalServerError, "failed on checkHashedPassword")
			return
		}
		if !checkResult {
			c.String(http.StatusForbidden, "not authorized")
			return
		}

		if err = setSession(c, userAttr.UserId); err != nil {
			c.String(http.StatusInternalServerError, "failed on encoding a cookie")
			return
		}

		c.String(http.StatusOK, "authorized")
	})

	r.GET("/logout", func(c *gin.Context) {
		_, err := getSession(c)

		if err != nil {
			c.String(http.StatusForbidden, "not logged in")
		} else {
			c.HTML(http.StatusOK, "logout.tmpl", gin.H{})
		}
	})
	r.POST("/logout", func(c *gin.Context) {
		expireSession(c)
		c.String(http.StatusOK, "logged out")
	})

	r.MaxMultipartMemory = 8 << 20 // 8 MiB
	r.POST("/upload", func(c *gin.Context) {
		userId, _ := getSession(c)
		if userId == "" {
			c.String(http.StatusForbidden, "unauthorized")
			return
		}

		// Source
		file, err := c.FormFile("file")
		if err != nil {
			c.String(http.StatusBadRequest, "get form err: %s", err.Error())
			return
		}

		filename := "data/uploaded/" + userId + "/" + filepath.Base(file.Filename)
		if err := c.SaveUploadedFile(file, filename); err != nil {
			c.String(http.StatusBadRequest, "upload file err: %s", err.Error())
			return
		}

		c.String(http.StatusOK, "File %s uploaded successfully.", file.Filename)
	})

	r.GET("/list", func(c *gin.Context) {
		userId, _ := getSession(c)
		if userId == "" {
			c.String(http.StatusForbidden, "unauthorized")
			return
		}

		entries, err := os.ReadDir("data/uploaded/" + userId)
		if err != nil {
			log.Fatal(err)
		}

		var files = []string{}
		for _, e := range entries {
			if e.Type().IsRegular() {
				files = append(files, e.Name())
			}
		}

		c.String(http.StatusOK, strings.Join(files, "\n"))
	})

	return r
}

func main() {
	cfg := mysql.Config{
		User:   os.Getenv("MYSQL_USER"),
		Passwd: os.Getenv("MYSQL_PASS"),
		Net:    "tcp",
		Addr:   os.Getenv("MYSQL_ADDR"),
		DBName: "vanxxxserver",
	}

	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		log.Fatal(err)
	}

	defer db.Close()

	// just testing
	return

	initSecureCookie()
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
