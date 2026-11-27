package domain

import "errors"

var (
	ErrNotFound   = errors.New("not found")
	ErrForbidden  = errors.New("forbidden")
	ErrConflict   = errors.New("conflict")
	ErrValidation = errors.New("validation error")

	// ErrUnavailable: внешний сервис отключён, перегружен, недоступен или превышен таймаут (HTTP 503).
	ErrUnavailable = errors.New("service unavailable")
	// ErrBadGateway: внешний сервис вернул невалидный ответ (HTTP 502).
	ErrBadGateway = errors.New("bad gateway")
)
