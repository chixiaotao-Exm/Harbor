package api

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func contextWithRoute(r *http.Request, rc *chi.Context) context.Context {
	return context.WithValue(r.Context(), chi.RouteCtxKey, rc)
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }
