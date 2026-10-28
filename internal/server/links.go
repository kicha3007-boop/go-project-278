package server

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"url-shortener/internal/links"
	"url-shortener/internal/store"
)

// linkPayload — тело создания и изменения ссылки; правила валидации — в тегах.
type linkPayload struct {
	OriginalURL string `json:"original_url" binding:"required,url"`
	ShortName   string `json:"short_name"   binding:"omitempty,min=3,max=32,shortname"`
}

type linkResponse struct {
	ID          int64     `json:"id"`
	OriginalURL string    `json:"original_url"`
	ShortName   string    `json:"short_name"`
	ShortURL    string    `json:"short_url"`
	CreatedAt   time.Time `json:"created_at"`
}

func (h *handler) toResponse(link store.Link) linkResponse {
	return linkResponse{
		ID:          link.ID,
		OriginalURL: link.OriginalUrl,
		ShortName:   link.ShortName,
		ShortURL:    h.baseURL + "/r/" + link.ShortName,
		CreatedAt:   link.CreatedAt.Time.UTC(),
	}
}

func (h *handler) listLinks(c *gin.Context) {
	page, err := parsePage(c)
	if err != nil {
		respondError(c, http.StatusBadRequest, err.Error())

		return
	}

	items, total, err := h.links.List(c.Request.Context(), page)
	if err != nil {
		internalError(c, err)

		return
	}

	body := make([]linkResponse, 0, len(items))
	for _, link := range items {
		body = append(body, h.toResponse(link))
	}

	c.Header("Content-Range", contentRange("links", page, len(body), total))
	c.JSON(http.StatusOK, body)
}

func (h *handler) createLink(c *gin.Context) {
	var payload linkPayload
	if !bindJSON(c, &payload) {
		return
	}

	link, err := h.links.Create(c.Request.Context(), payload.OriginalURL, payload.ShortName)
	if err != nil {
		h.respondLinkError(c, err)

		return
	}

	c.JSON(http.StatusCreated, h.toResponse(link))
}

func (h *handler) getLink(c *gin.Context) {
	id, ok := linkID(c)
	if !ok {
		return
	}

	link, err := h.links.Get(c.Request.Context(), id)
	if err != nil {
		h.respondLinkError(c, err)

		return
	}

	c.JSON(http.StatusOK, h.toResponse(link))
}

func (h *handler) updateLink(c *gin.Context) {
	id, ok := linkID(c)
	if !ok {
		return
	}

	var payload linkPayload
	if !bindJSON(c, &payload) {
		return
	}

	link, err := h.links.Update(c.Request.Context(), id, payload.OriginalURL, payload.ShortName)
	if err != nil {
		h.respondLinkError(c, err)

		return
	}

	c.JSON(http.StatusOK, h.toResponse(link))
}

func (h *handler) deleteLink(c *gin.Context) {
	id, ok := linkID(c)
	if !ok {
		return
	}

	if err := h.links.Delete(c.Request.Context(), id); err != nil {
		h.respondLinkError(c, err)

		return
	}

	c.Status(http.StatusNoContent)
}

// linkID читает :id; на неверный id сразу отвечает 404 — такой ссылки быть не может.
func linkID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		respondError(c, http.StatusNotFound, links.ErrNotFound.Error())

		return 0, false
	}

	return id, true
}

// respondLinkError переводит ошибки пакета links в ответы API.
func (h *handler) respondLinkError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, links.ErrNotFound):
		respondError(c, http.StatusNotFound, err.Error())
	case errors.Is(err, links.ErrShortNameTaken):
		respondValidation(c, map[string]string{"short_name": err.Error()})
	default:
		internalError(c, err)
	}
}
