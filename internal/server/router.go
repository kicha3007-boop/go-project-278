// Package server — HTTP-слой: маршруты, обработчики, единый формат ошибок.
package server

import (
	"fmt"
	"log"
	"net/http"

	"github.com/getsentry/sentry-go"
	sentrygin "github.com/getsentry/sentry-go/gin"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"

	"url-shortener/internal/config"
	"url-shortener/internal/links"
)

// handler держит зависимости обработчиков.
type handler struct {
	links   *links.Service
	baseURL string
}

// NewRouter собирает все маршруты приложения в одном месте.
func NewRouter(cfg config.Config, service *links.Service) (*gin.Engine, error) {
	registerValidators()

	router := gin.New()
	// Render стоит за Cloudflare: настоящий IP клиента — в CF-Connecting-IP.
	// Без этого заголовка Gin берёт X-Forwarded-For от доверенных прокси (Caddy в том же контейнере).
	router.TrustedPlatform = gin.PlatformCloudflare
	if err := router.SetTrustedProxies(cfg.TrustedProxies); err != nil {
		return nil, fmt.Errorf("trusted proxies: %w", err)
	}

	router.Use(gin.Logger(), gin.Recovery())
	if cfg.SentryDSN != "" {
		router.Use(sentrygin.New(sentrygin.Options{Repanic: true}))
	}

	router.Use(cors.New(cors.Config{
		AllowOrigins:  cfg.CORSOrigins,
		AllowMethods:  []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions},
		AllowHeaders:  []string{"Content-Type", "Range"},
		ExposeHeaders: []string{"Content-Range"},
	}))

	h := &handler{links: service, baseURL: cfg.BaseURL}

	router.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	router.GET("/r/:code", h.redirect)

	api := router.Group("/api")
	api.GET("/links", h.listLinks)
	api.POST("/links", h.createLink)
	api.GET("/links/:id", h.getLink)
	api.PUT("/links/:id", h.updateLink)
	api.DELETE("/links/:id", h.deleteLink)
	api.GET("/link_visits", h.listVisits)

	if cfg.EnableDebugRoutes {
		api.GET("/debug/error", func(c *gin.Context) {
			internalError(c, fmt.Errorf("debug error for monitoring check"))
		})
	}

	router.NoRoute(func(c *gin.Context) { respondError(c, http.StatusNotFound, "not found") })

	return router, nil
}

// respondError — единый формат ошибки: {"error": "..."}.
func respondError(c *gin.Context, status int, message string) {
	c.AbortWithStatusJSON(status, gin.H{"error": message})
}

// internalError отдаёт 500 и отправляет ошибку в мониторинг.
func internalError(c *gin.Context, err error) {
	captureError(c, err)
	respondError(c, http.StatusInternalServerError, "internal server error")
}

// captureError пишет ошибку в лог и отправляет в мониторинг (без SENTRY_DSN — только лог).
func captureError(c *gin.Context, err error) {
	log.Printf("%s %s: %v", c.Request.Method, c.Request.URL.Path, err)

	if hub := sentrygin.GetHubFromContext(c); hub != nil {
		hub.CaptureException(err)

		return
	}

	sentry.CaptureException(err)
}
