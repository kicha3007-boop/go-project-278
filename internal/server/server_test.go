package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"

	"url-shortener/db"
	"url-shortener/internal/config"
	"url-shortener/internal/links"
)

const baseURL = "http://short.test"

// testPool — отдельная тестовая база из TEST_DATABASE_URL (не база приложения).
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		fmt.Println("TEST_DATABASE_URL не задан: тесты API пропущены (см. README, make test)")
		os.Exit(0)
	}

	gin.SetMode(gin.TestMode)

	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	provider, err := goose.NewProvider(goose.DialectPostgres, stdlib.OpenDBFromPool(pool), db.MigrationsFS())
	if err == nil {
		_, err = provider.Up(context.Background())
	}

	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	testPool = pool
	code := m.Run()
	pool.Close()
	os.Exit(code)
}

// newRouter приводит базу в пустое состояние и собирает роутер.
func newRouter(t *testing.T) *gin.Engine {
	t.Helper()

	_, err := testPool.Exec(context.Background(), "TRUNCATE link_visits, links RESTART IDENTITY CASCADE")
	require.NoError(t, err)

	router, err := NewRouter(config.Config{
		BaseURL:        baseURL,
		CORSOrigins:    []string{"http://localhost:5173"},
		TrustedProxies: []string{"127.0.0.1"},
	}, links.NewService(testPool))
	require.NoError(t, err)

	return router
}

func request(t *testing.T, router http.Handler, method, path, body string, headers ...string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}

	for i := 0; i+1 < len(headers); i += 2 {
		req.Header.Set(headers[i], headers[i+1])
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()

	var value T
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &value), rec.Body.String())

	return value
}

func createLink(t *testing.T, router http.Handler, originalURL, shortName string) linkResponse {
	t.Helper()

	rec := request(t, router, http.MethodPost, "/api/links",
		fmt.Sprintf(`{"original_url": %q, "short_name": %q}`, originalURL, shortName))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	return decode[linkResponse](t, rec)
}

func TestPing(t *testing.T) {
	rec := request(t, newRouter(t), http.MethodGet, "/ping", "")

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "pong", rec.Body.String())
}

func TestUnknownRoute(t *testing.T) {
	rec := request(t, newRouter(t), http.MethodGet, "/nope", "")

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, map[string]string{"error": "not found"}, decode[map[string]string](t, rec))
}

func TestCreateLink(t *testing.T) {
	router := newRouter(t)

	rec := request(t, router, http.MethodPost, "/api/links", `{"original_url": "https://example.com/long", "short_name": "exmpl"}`)
	require.Equal(t, http.StatusCreated, rec.Code)
	require.Equal(t, "application/json; charset=utf-8", rec.Header().Get("Content-Type"))

	link := decode[linkResponse](t, rec)
	require.Equal(t, int64(1), link.ID)
	require.Equal(t, "https://example.com/long", link.OriginalURL)
	require.Equal(t, "exmpl", link.ShortName)
	require.Equal(t, baseURL+"/r/exmpl", link.ShortURL)
	require.False(t, link.CreatedAt.IsZero())
}

func TestCreateLinkGeneratesShortName(t *testing.T) {
	router := newRouter(t)

	first := createLink(t, router, "https://example.com/1", "")
	second := createLink(t, router, "https://example.com/2", "")

	require.Len(t, first.ShortName, 8)
	require.NotEqual(t, first.ShortName, second.ShortName)
	require.Equal(t, baseURL+"/r/"+first.ShortName, first.ShortURL)
}

func TestCreateLinkValidation(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		field string
	}{
		{name: "empty body", body: `{}`, field: "original_url"},
		{name: "not url", body: `{"original_url": "not a url"}`, field: "original_url"},
		{name: "short name too short", body: `{"original_url": "https://example.com", "short_name": "ab"}`, field: "short_name"},
		{name: "short name too long", body: `{"original_url": "https://example.com", "short_name": "` + strings.Repeat("a", 33) + `"}`, field: "short_name"},
		{name: "short name with spaces", body: `{"original_url": "https://example.com", "short_name": "a b c"}`, field: "short_name"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := request(t, newRouter(t), http.MethodPost, "/api/links", tt.body)
			require.Equal(t, http.StatusUnprocessableEntity, rec.Code)

			body := decode[map[string]map[string]string](t, rec)
			require.Contains(t, body["errors"], tt.field)
		})
	}
}

func TestCreateLinkMalformedJSON(t *testing.T) {
	rec := request(t, newRouter(t), http.MethodPost, "/api/links", "not-json")

	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, map[string]string{"error": "invalid request"}, decode[map[string]string](t, rec))
}

func TestCreateLinkDuplicateShortName(t *testing.T) {
	router := newRouter(t)
	createLink(t, router, "https://example.com/1", "taken")

	rec := request(t, router, http.MethodPost, "/api/links", `{"original_url": "https://example.com/2", "short_name": "taken"}`)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
	require.Equal(t,
		map[string]map[string]string{"errors": {"short_name": "short name already in use"}},
		decode[map[string]map[string]string](t, rec))
}

func TestGetLink(t *testing.T) {
	router := newRouter(t)
	created := createLink(t, router, "https://example.com/get", "getme")

	rec := request(t, router, http.MethodGet, fmt.Sprintf("/api/links/%d", created.ID), "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, created, decode[linkResponse](t, rec))

	for _, path := range []string{"/api/links/999", "/api/links/abc"} {
		rec = request(t, router, http.MethodGet, path, "")
		require.Equal(t, http.StatusNotFound, rec.Code, path)
		require.Equal(t, map[string]string{"error": "link not found"}, decode[map[string]string](t, rec))
	}
}

func TestUpdateLink(t *testing.T) {
	router := newRouter(t)
	created := createLink(t, router, "https://example.com/old", "old-name")
	createLink(t, router, "https://example.com/other", "other")
	path := fmt.Sprintf("/api/links/%d", created.ID)

	rec := request(t, router, http.MethodPut, path, `{"original_url": "https://example.com/new", "short_name": "new-name"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	updated := decode[linkResponse](t, rec)
	require.Equal(t, created.ID, updated.ID)
	require.Equal(t, "https://example.com/new", updated.OriginalURL)
	require.Equal(t, baseURL+"/r/new-name", updated.ShortURL)

	rec = request(t, router, http.MethodPut, path, `{"original_url": "https://example.com/newer"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "new-name", decode[linkResponse](t, rec).ShortName)

	rec = request(t, router, http.MethodPut, path, `{"original_url": "https://example.com/x", "short_name": "other"}`)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	rec = request(t, router, http.MethodPut, path, "not-json")
	require.Equal(t, http.StatusBadRequest, rec.Code)

	rec = request(t, router, http.MethodPut, path, `{"original_url": ""}`)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)

	rec = request(t, router, http.MethodPut, "/api/links/999", `{"original_url": "https://example.com/x"}`)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, map[string]string{"error": "link not found"}, decode[map[string]string](t, rec))
}

func TestDeleteLink(t *testing.T) {
	router := newRouter(t)
	created := createLink(t, router, "https://example.com/del", "delete-me")
	path := fmt.Sprintf("/api/links/%d", created.ID)

	rec := request(t, router, http.MethodDelete, path, "")
	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Empty(t, rec.Body.String())

	rec = request(t, router, http.MethodGet, path, "")
	require.Equal(t, http.StatusNotFound, rec.Code)

	rec = request(t, router, http.MethodDelete, path, "")
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, map[string]string{"error": "link not found"}, decode[map[string]string](t, rec))
}

func TestListLinksPagination(t *testing.T) {
	router := newRouter(t)

	rec := request(t, router, http.MethodGet, "/api/links", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "links */0", rec.Header().Get("Content-Range"))
	require.Empty(t, decode[[]linkResponse](t, rec))

	for i := range 12 {
		createLink(t, router, fmt.Sprintf("https://example.com/%d", i), fmt.Sprintf("name-%02d", i))
	}

	tests := []struct {
		name      string
		query     string
		header    string
		wantRange string
		wantIDs   []int64
	}{
		{name: "all", query: "", wantRange: "links 0-11/12", wantIDs: ids(1, 12)},
		{name: "first page", query: "?range=[0,4]", wantRange: "links 0-4/12", wantIDs: ids(1, 5)},
		{name: "spaces", query: "?range=[5,%209]", wantRange: "links 5-9/12", wantIDs: ids(6, 10)},
		{name: "last page", query: "?range=[10,14]", wantRange: "links 10-11/12", wantIDs: ids(11, 12)},
		{name: "out of bounds", query: "?range=[20,29]", wantRange: "links */12", wantIDs: []int64{}},
		{name: "range header", header: "[2,3]", wantRange: "links 2-3/12", wantIDs: ids(3, 4)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var headers []string
			if tt.header != "" {
				headers = []string{"Range", tt.header}
			}

			rec := request(t, router, http.MethodGet, "/api/links"+tt.query, "", headers...)
			require.Equal(t, http.StatusOK, rec.Code)
			require.Equal(t, tt.wantRange, rec.Header().Get("Content-Range"))

			got := []int64{}
			for _, link := range decode[[]linkResponse](t, rec) {
				got = append(got, link.ID)
			}

			require.Equal(t, tt.wantIDs, got)
		})
	}

	for _, bad := range []string{"[1]", "[5,2]", "[-1,3]", "abc"} {
		rec := request(t, router, http.MethodGet, "/api/links?range="+bad, "")
		require.Equal(t, http.StatusBadRequest, rec.Code, bad)
		require.Equal(t, map[string]string{"error": "invalid range"}, decode[map[string]string](t, rec))
	}
}

func TestRedirectRecordsVisit(t *testing.T) {
	router := newRouter(t)
	created := createLink(t, router, "https://example.com/target", "go-there")

	req := httptest.NewRequest(http.MethodGet, "/r/go-there", nil)
	req.RemoteAddr = "127.0.0.1:5000" // запрос пришёл через доверенный прокси
	req.Header.Set("X-Forwarded-For", "203.0.113.7")
	req.Header.Set("User-Agent", "test-agent")
	req.Header.Set("Referer", "https://ref.example")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusFound, rec.Code)
	require.Equal(t, "https://example.com/target", rec.Header().Get("Location"))

	rec = request(t, router, http.MethodGet, "/api/link_visits?range=[0,9]", "")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "link_visits 0-0/1", rec.Header().Get("Content-Range"))

	visits := decode[[]visitResponse](t, rec)
	require.Len(t, visits, 1)
	require.Equal(t, created.ID, visits[0].LinkID)
	require.Equal(t, "203.0.113.7", visits[0].IP)
	require.Equal(t, "test-agent", visits[0].UserAgent)
	require.Equal(t, "https://ref.example", visits[0].Referer)
	require.Equal(t, int32(http.StatusFound), visits[0].Status)
	require.False(t, visits[0].CreatedAt.IsZero())
}

func TestRedirectUnknownCode(t *testing.T) {
	rec := request(t, newRouter(t), http.MethodGet, "/r/missing", "")

	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, map[string]string{"error": "link not found"}, decode[map[string]string](t, rec))
}

func TestListVisitsEmpty(t *testing.T) {
	rec := request(t, newRouter(t), http.MethodGet, "/api/link_visits", "", "Range", "[0,9]")

	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "link_visits */0", rec.Header().Get("Content-Range"))
	require.Empty(t, decode[[]visitResponse](t, rec))
}

func TestCORS(t *testing.T) {
	rec := request(t, newRouter(t), http.MethodOptions, "/api/links", "",
		"Origin", "http://localhost:5173", "Access-Control-Request-Method", "PUT")

	require.Equal(t, http.StatusNoContent, rec.Code)
	require.Equal(t, "http://localhost:5173", rec.Header().Get("Access-Control-Allow-Origin"))
}

func ids(from, to int64) []int64 {
	result := []int64{}
	for id := from; id <= to; id++ {
		result = append(result, id)
	}

	return result
}
