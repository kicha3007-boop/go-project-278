// Package links — работа с короткими ссылками и их посещениями поверх запросов sqlc.
package links

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"url-shortener/internal/store"
)

const (
	uniqueViolation     = "23505"
	shortNameConstraint = "links_short_name_key"
	generatedNameLength = 8
	generateAttempts    = 5
)

var (
	// ErrNotFound — ссылки с таким id или коротким именем нет.
	ErrNotFound = errors.New("link not found")
	// ErrShortNameTaken — короткое имя уже занято другой ссылкой.
	ErrShortNameTaken = errors.New("short name already in use")
)

// Page — диапазон выборки: Limit nil означает «до конца».
type Page struct {
	Offset int32
	Limit  *int32
}

// Service — операции над ссылками.
type Service struct {
	queries *store.Queries
}

// NewService создаёт Service поверх подключения к базе.
func NewService(db store.DBTX) *Service {
	return &Service{queries: store.New(db)}
}

// Create сохраняет ссылку; пустой shortName генерируется.
func (s *Service) Create(ctx context.Context, originalURL, shortName string) (store.Link, error) {
	if shortName != "" {
		link, err := s.queries.CreateLink(ctx, store.CreateLinkParams{OriginalUrl: originalURL, ShortName: shortName})

		return link, wrap(err, "create link")
	}

	// Сгенерированное имя может совпасть с занятым — пробуем ещё раз.
	for range generateAttempts {
		link, err := s.queries.CreateLink(ctx, store.CreateLinkParams{OriginalUrl: originalURL, ShortName: generateName()})
		if !errors.Is(wrap(err, ""), ErrShortNameTaken) {
			return link, wrap(err, "create link")
		}
	}

	return store.Link{}, fmt.Errorf("create link: no free short name after %d attempts", generateAttempts)
}

// Get возвращает ссылку по id.
func (s *Service) Get(ctx context.Context, id int64) (store.Link, error) {
	link, err := s.queries.GetLink(ctx, id)

	return link, wrap(err, "get link")
}

// Update меняет адрес и короткое имя ссылки; пустой shortName оставляет прежнее.
func (s *Service) Update(ctx context.Context, id int64, originalURL, shortName string) (store.Link, error) {
	if shortName == "" {
		current, err := s.Get(ctx, id)
		if err != nil {
			return store.Link{}, err
		}

		shortName = current.ShortName
	}

	link, err := s.queries.UpdateLink(ctx, store.UpdateLinkParams{ID: id, OriginalUrl: originalURL, ShortName: shortName})

	return link, wrap(err, "update link")
}

// Delete удаляет ссылку вместе с её посещениями.
func (s *Service) Delete(ctx context.Context, id int64) error {
	deleted, err := s.queries.DeleteLink(ctx, id)
	if err != nil {
		return fmt.Errorf("delete link: %w", err)
	}

	if deleted == 0 {
		return ErrNotFound
	}

	return nil
}

// List возвращает страницу ссылок и общее их число.
func (s *Service) List(ctx context.Context, page Page) ([]store.Link, int64, error) {
	items, err := s.queries.ListLinks(ctx, store.ListLinksParams{PageOffset: page.Offset, PageLimit: page.Limit})
	if err != nil {
		return nil, 0, fmt.Errorf("list links: %w", err)
	}

	total, err := s.queries.CountLinks(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count links: %w", err)
	}

	return items, total, nil
}

// Resolve находит ссылку по короткому имени.
func (s *Service) Resolve(ctx context.Context, shortName string) (store.Link, error) {
	link, err := s.queries.GetLinkByShortName(ctx, shortName)

	return link, wrap(err, "resolve link")
}

// Visit — данные о переходе по короткой ссылке.
type Visit struct {
	LinkID    int64
	IP        string
	UserAgent string
	Referer   string
	Status    int32
}

// RecordVisit сохраняет переход по ссылке.
func (s *Service) RecordVisit(ctx context.Context, visit Visit) error {
	_, err := s.queries.CreateLinkVisit(ctx, store.CreateLinkVisitParams{
		LinkID:    visit.LinkID,
		Ip:        visit.IP,
		UserAgent: visit.UserAgent,
		Referer:   visit.Referer,
		Status:    visit.Status,
	})
	if err != nil {
		return fmt.Errorf("record visit: %w", err)
	}

	return nil
}

// ListVisits возвращает страницу посещений и общее их число.
func (s *Service) ListVisits(ctx context.Context, page Page) ([]store.LinkVisit, int64, error) {
	items, err := s.queries.ListLinkVisits(ctx, store.ListLinkVisitsParams{PageOffset: page.Offset, PageLimit: page.Limit})
	if err != nil {
		return nil, 0, fmt.Errorf("list visits: %w", err)
	}

	total, err := s.queries.CountLinkVisits(ctx)
	if err != nil {
		return nil, 0, fmt.Errorf("count visits: %w", err)
	}

	return items, total, nil
}

// wrap переводит ошибки базы в ошибки пакета, остальные дополняет контекстом.
func wrap(err error, op string) error {
	var pgErr *pgconn.PgError

	switch {
	case err == nil:
		return nil
	case errors.Is(err, pgx.ErrNoRows):
		return ErrNotFound
	case errors.As(err, &pgErr) && pgErr.Code == uniqueViolation && pgErr.ConstraintName == shortNameConstraint:
		return ErrShortNameTaken
	default:
		return fmt.Errorf("%s: %w", op, err)
	}
}

// generateName — случайное короткое имя (base32: заглавные латинские буквы и цифры).
func generateName() string {
	return rand.Text()[:generatedNameLength]
}
