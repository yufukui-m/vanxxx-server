package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

const (
	SESSION_COOKIE_NAME = "session"
)

type Request struct {
	AnthropicVersion string    `json:"anthropic_version"`
	MaxTokens        int       `json:"max_tokens"`
	System           string    `json:"system"`
	Messages         []Message `json:"messages"`
}

type Message struct {
	Role    string    `json:"role"`
	Content []Content `json:"content"`
}

type Content struct {
	Type   string  `json:"type"`
	Text   string  `json:"text,omitempty"`
	Source *Source `json:"source,omitempty"`
}

type Source struct {
	Type      string `json:"type"`
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

type Response struct {
	ID           string        `json:"id"`
	Model        string        `json:"model"`
	Type         string        `json:"type"`
	Role         string        `json:"role"`
	ContentItem  []ContentItem `json:"content"`
	StopReason   string        `json:"stop_reason,omitempty"`
	StopSequence string        `json:"stop_sequence,omitempty"`
	Usage        UsageDetails  `json:"usage"`
}

type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type UsageDetails struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

func isItShibuya(base64Image string) (string, error) {
	SYSTEM_PROMPT := "この画像の渋谷っぽいところを教えてください。回答の最後に渋谷である確率をパーセントで教えてください。回答は日本語でお願いします。"

	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return "", err
	}

	bedrock := bedrockruntime.NewFromConfig(cfg)

	contentImage := Content{
		Type: "image",
		Source: &Source{
			Type:      "base64",
			MediaType: "image/jpeg",
			Data:      base64Image,
		},
	}

	message := Message{
		Role:    "user",
		Content: []Content{contentImage},
	}

	payload := Request{
		AnthropicVersion: "bedrock-2023-05-31", // TODO
		MaxTokens:        1024,
		System:           SYSTEM_PROMPT,
		Messages:         []Message{message},
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	output, err := bedrock.InvokeModel(context.Background(), &bedrockruntime.InvokeModelInput{
		Body:        payloadBytes,
		ModelId:     aws.String("anthropic.claude-3-haiku-20240307-v1:0"),
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return "", err
	}

	var resp Response

	err = json.Unmarshal(output.Body, &resp)
	if err != nil {
		return "", err
	}

	return resp.ContentItem[len(resp.ContentItem)-1].Text, nil
}

func setupRouter() *gin.Engine {
	r := gin.Default()
	r.LoadHTMLGlob("templates/*")

	corsConfig := cors.DefaultConfig()
	corsConfig.AllowOrigins = []string{"http://localhost:3000"}
	corsConfig.AllowCredentials = true
	r.Use(cors.New(corsConfig))

	// Ping test
	r.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	r.GET("/", func(c *gin.Context) {
		c.String(http.StatusOK, "hello")
	})

	r.POST("/api/uploadImage", func(c *gin.Context) {
		imageBlob, _, err := c.Request.FormFile("image")
		if err != nil {
			c.JSON(400, gin.H{
				"message": "Bad Request",
			})
			return
		}
		/*
			var buff bytes.Buffer
			b64encoder := base64.NewEncoder(base64.StdEncoding, buff)
			b64encoder.Write(imageBlob.)
			b64encoder.Close()*/

		var buff bytes.Buffer
		buff.ReadFrom(imageBlob)
		b64string := base64.StdEncoding.EncodeToString(buff.Bytes())

		result, err := isItShibuya(b64string)
		if err != nil {
			c.JSON(500, gin.H{
				"message": err.Error(),
			})
			return
		}

		c.JSON(200, gin.H{
			"message": result,
		})
	})

	return r
}

func main() {
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

	log.Println("Server exiting")
}
