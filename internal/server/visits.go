package server

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"url-shortener/internal/links"
)

type visitResponse struct {
	ID        int64     `json:"id"`
	LinkID    int64     `json:"link_id"`
	CreatedAt time.Time `json:"created_at"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
	Referer   string    `json:"referer"`
	Status    int32     `json:"status"`
}

// redirect переводит по короткому имени на исходный адрес и записывает посещение.
// Ошибка записи посещения уходит в мониторинг, но переход не ломает.
func (h *handler) redirect(c *gin.Context) {
	ctx := c.Request.Context()

	link, err := h.links.Resolve(ctx, c.Param("code"))
	if err != nil {
		if errors.Is(err, links.ErrNotFound) {
			respondError(c, http.StatusNotFound, err.Error())
		} else {
			internalError(c, err)
		}

		return
	}

	const status = http.StatusFound

	visit := links.Visit{
		LinkID:    link.ID,
		IP:        c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		Referer:   c.Request.Referer(),
		Status:    status,
	}
	if err := h.links.RecordVisit(ctx, visit); err != nil {
		captureError(c, fmt.Errorf("redirect %s: %w", link.ShortName, err))
	}

	c.Redirect(status, link.OriginalUrl)
}

func (h *handler) listVisits(c *gin.Context) {
	page, err := parsePage(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())

		return
	}

	items, total, err := h.links.ListVisits(c.Request.Context(), page)
	if err != nil {
		internalError(c, err)

		return
	}

	body := make([]visitResponse, 0, len(items))
	for _, visit := range items {
		body = append(body, visitResponse{
			ID:        visit.ID,
			LinkID:    visit.LinkID,
			CreatedAt: visit.CreatedAt.Time.UTC(),
			IP:        visit.Ip,
			UserAgent: visit.UserAgent,
			Referer:   visit.Referer,
			Status:    visit.Status,
		})
	}

	c.Header("Content-Range", contentRange("link_visits", page, len(body), total))
	c.JSON(http.StatusOK, body)
}
