package server

import (
	"errors"
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
)

// shortNamePattern — короткое имя попадает в URL, поэтому без пробелов и спецсимволов.
var shortNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

var registerOnce sync.Once

// registerValidators учит валидатор Gin правилу shortname и именам полей из json-тегов.
func registerValidators() {
	registerOnce.Do(func() {
		engine, ok := binding.Validator.Engine().(*validator.Validate)
		if !ok {
			return
		}

		engine.RegisterTagNameFunc(func(field reflect.StructField) string {
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			if name == "" || name == "-" {
				return field.Name
			}

			return name
		})

		err := engine.RegisterValidation("shortname", func(fl validator.FieldLevel) bool {
			return shortNamePattern.MatchString(fl.Field().String())
		})
		if err != nil {
			panic(err) // имя правила зашито в код: ошибка возможна только при правке кода
		}
	})
}

// bindJSON разбирает тело запроса в payload. Ошибку оно уже отдало клиенту, если вернуло false:
// 400 — тело не разобрать, 422 — не прошли правила валидации.
func bindJSON(c *gin.Context, payload any) bool {
	err := c.ShouldBindJSON(payload)
	if err == nil {
		return true
	}

	var validationErrors validator.ValidationErrors
	if !errors.As(err, &validationErrors) {
		respondError(c, http.StatusBadRequest, "invalid request")

		return false
	}

	fields := make(map[string]string, len(validationErrors))
	for _, fieldErr := range validationErrors {
		fields[fieldErr.Field()] = fieldErr.Error()
	}

	respondValidation(c, fields)

	return false
}

// respondValidation — единый формат ошибок валидации: 422 {"errors": {"<поле>": "<сообщение>"}}.
func respondValidation(c *gin.Context, fields map[string]string) {
	c.AbortWithStatusJSON(http.StatusUnprocessableEntity, gin.H{"errors": fields})
}
