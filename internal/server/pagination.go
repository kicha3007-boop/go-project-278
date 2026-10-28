package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/gin-gonic/gin"

	"url-shortener/internal/links"
)

var errInvalidRange = errors.New("invalid range")

// parsePage читает диапазон ra-data-simple-rest: ?range=[start,end] или заголовок Range: [start,end].
// Границы включительно: первые 10 записей — [0,9]. Без диапазона — вся коллекция.
func parsePage(c *gin.Context) (links.Page, error) {
	raw := c.Query("range")
	if raw == "" {
		raw = c.GetHeader("Range")
	}

	if raw == "" {
		return links.Page{}, nil
	}

	var bounds []int64
	if err := json.Unmarshal([]byte(raw), &bounds); err != nil || len(bounds) != 2 {
		return links.Page{}, errInvalidRange
	}

	start, end := bounds[0], bounds[1]
	if start < 0 || end < start || end >= math.MaxInt32 {
		return links.Page{}, errInvalidRange
	}

	limit := int32(end - start + 1)

	return links.Page{Offset: int32(start), Limit: &limit}, nil
}

// contentRange — заголовок Content-Range с реально отданными позициями (включительно) и общим числом.
func contentRange(resource string, page links.Page, returned int, total int64) string {
	if returned == 0 {
		return fmt.Sprintf("%s */%d", resource, total)
	}

	return fmt.Sprintf("%s %d-%d/%d", resource, page.Offset, int(page.Offset)+returned-1, total)
}
