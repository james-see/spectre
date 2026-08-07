package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"

	"github.com/james-see/spectre/services/deco/docs"
	"github.com/james-see/spectre/services/deco/internal/deco"
)

func main() {
	// Overload so a stale exported DECO_USER in the shell cannot override .env.
	_ = godotenv.Overload()
	_ = godotenv.Overload("../../.env")

	host := env("DECO_HOST", "https://192.168.68.1")
	user := env("DECO_USER", "admin")
	pass := os.Getenv("DECO_PASS")
	bind := env("DECO_BIND", "127.0.0.1:3002")
	pollSec, _ := strconv.Atoi(env("DECO_POLL_SECONDS", "3"))
	if pollSec < 1 {
		pollSec = 3
	}

	client := deco.NewClient(host, user, pass)
	poller := deco.NewPoller(client, time.Duration(pollSec)*time.Second)
	if pass == "" {
		log.Printf("warning: DECO_PASS empty — polls will fail closed until set")
	}
	poller.Start()

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery(), gin.Logger())

	r.GET("/health", healthHandler(poller))
	r.GET("/api/lan/mesh", meshHandler(poller))
	r.GET("/api/lan/clients", clientsHandler(poller))
	r.GET("/openapi.json", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json", docs.OpenAPI)
	})
	r.GET("/swagger", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/openapi.json")
	})

	log.Printf("spectre-deco listening on http://%s (host=%s)", bind, host)
	if err := r.Run(bind); err != nil {
		log.Fatal(err)
	}
}

func healthHandler(poller *deco.Poller) gin.HandlerFunc {
	return func(c *gin.Context) {
		snap := poller.Snapshot()
		status := "ok"
		if snap.Error != "" {
			status = "degraded"
		}
		c.JSON(http.StatusOK, gin.H{
			"status":     status,
			"service":    "spectre-deco",
			"error":      snap.Error,
			"updated_at": snap.UpdatedAt,
			"mesh":       len(snap.Mesh),
			"clients":    len(snap.Clients),
		})
	}
}

func meshHandler(poller *deco.Poller) gin.HandlerFunc {
	return func(c *gin.Context) {
		snap := poller.Snapshot()
		mesh := snap.Mesh
		if mesh == nil {
			mesh = []deco.MeshNode{}
		}
		c.JSON(http.StatusOK, gin.H{
			"mesh":       mesh,
			"error":      snap.Error,
			"updated_at": snap.UpdatedAt,
		})
	}
}

func clientsHandler(poller *deco.Poller) gin.HandlerFunc {
	return func(c *gin.Context) {
		snap := poller.Snapshot()
		clients := snap.Clients
		if clients == nil {
			clients = []deco.LanClient{}
		}
		c.JSON(http.StatusOK, gin.H{
			"clients":    clients,
			"error":      snap.Error,
			"updated_at": snap.UpdatedAt,
		})
	}
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
