package server

import (
	"gin-ddb/internal/config"
	"gin-ddb/internal/handlers"
	"gin-ddb/internal/service"

	"github.com/gin-gonic/gin"
)

// New builds the fully wired engine: middleware plus the §4 routes.
func New(cfg *config.Config, svc *service.Service) *gin.Engine {
	e := NewBase(cfg)
	// Dates in :metadata arrive %2F-encoded; match on the raw path so an
	// encoded slash never splits into extra segments (params unescape).
	e.UseRawPath = true
	e.UnescapePathValues = true
	handlers.RegisterRoutes(e, handlers.New(svc, cfg))
	return e
}
