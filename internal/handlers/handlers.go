// Package handlers implements the §4 route table: binding via schemas,
// domain calls via service, and the §9 envelope on every outcome.
package handlers

import (
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"gin-ddb/internal/apperrors"
	"gin-ddb/internal/config"
	_ "gin-ddb/internal/docs" // generated OpenAPI spec served at /docs
	"gin-ddb/internal/models"
	"gin-ddb/internal/pagination"
	"gin-ddb/internal/schemas"
	"gin-ddb/internal/service"
)

// Handler serves the SkiLifts API.
type Handler struct {
	svc     *service.Service
	cfg     *config.Config
	started time.Time
}

// New returns a Handler wired to svc and cfg (cfg supplies table/region
// for GET / and rate-limit-unrelated route behavior).
func New(svc *service.Service, cfg *config.Config) *Handler {
	return &Handler{svc: svc, cfg: cfg, started: time.Now()}
}

func writeServiceError(c *gin.Context, serr *service.Error) {
	apperrors.Write(c, serr.Code, serr.Status, serr.Message, serr.Details)
}

func readBody(c *gin.Context) ([]byte, bool) {
	raw, err := io.ReadAll(c.Request.Body)
	if err != nil {
		apperrors.Validation(c, "invalid request body",
			[]apperrors.Detail{{Field: "body", Message: "invalid JSON"}})
		return nil, false
	}
	return raw, true
}

// CreateStatic godoc
// @Summary Create a static lift record
// @Accept json
// @Produce json
// @Param body body object true "static record"
// @Success 201 {object} object
// @Failure 400 {object} object
// @Failure 409 {object} object
// @Router /api/skilifts/static [post]
func (h *Handler) CreateStatic(c *gin.Context) {
	raw, ok := readBody(c)
	if !ok {
		return
	}
	rec, details := schemas.DecodeCreate(schemas.KindStatic, raw)
	if len(details) > 0 {
		apperrors.Validation(c, "invalid static record", details)
		return
	}
	created, serr := h.svc.CreateStatic(c.Request.Context(), rec.(models.StaticRecord))
	if serr != nil {
		writeServiceError(c, serr)
		return
	}
	c.JSON(http.StatusCreated, created)
}

// CreateDynamic godoc
// @Summary Create a dynamic daily record
// @Accept json
// @Produce json
// @Param body body object true "dynamic record"
// @Success 201 {object} object
// @Failure 400 {object} object
// @Failure 409 {object} object
// @Router /api/skilifts/dynamic [post]
func (h *Handler) CreateDynamic(c *gin.Context) {
	raw, ok := readBody(c)
	if !ok {
		return
	}
	rec, details := schemas.DecodeCreate(schemas.KindDynamic, raw)
	if len(details) > 0 {
		apperrors.Validation(c, "invalid dynamic record", details)
		return
	}
	created, serr := h.svc.CreateDynamic(c.Request.Context(), rec.(models.DynamicRecord))
	if serr != nil {
		writeServiceError(c, serr)
		return
	}
	c.JSON(http.StatusCreated, created)
}

// CreateResort godoc
// @Summary Create a resort aggregate record
// @Accept json
// @Produce json
// @Param body body object true "resort record"
// @Success 201 {object} object
// @Failure 400 {object} object
// @Failure 409 {object} object
// @Router /api/skilifts/resort [post]
func (h *Handler) CreateResort(c *gin.Context) {
	raw, ok := readBody(c)
	if !ok {
		return
	}
	rec, details := schemas.DecodeCreate(schemas.KindResort, raw)
	if len(details) > 0 {
		apperrors.Validation(c, "invalid resort record", details)
		return
	}
	created, serr := h.svc.CreateResort(c.Request.Context(), rec.(models.ResortRecord))
	if serr != nil {
		writeServiceError(c, serr)
		return
	}
	c.JSON(http.StatusCreated, created)
}

// collection renders {items,nextToken?,lastEvaluatedKey?,count}.
func (h *Handler) collection(c *gin.Context, page service.Page, serr *service.Error) {
	if serr != nil {
		writeServiceError(c, serr)
		return
	}
	items := page.Items
	if items == nil {
		items = []map[string]any{}
	}
	out := gin.H{"items": items, "count": page.Count}
	if page.HasNext {
		token, err := pagination.Encode(page.LastEvaluatedKey)
		if err == nil {
			out["nextToken"] = token
		}
		out["lastEvaluatedKey"] = page.LastEvaluatedKey
	}
	c.JSON(http.StatusOK, out)
}

// ListAll godoc
// @Summary List all records (scan, paginated)
// @Produce json
// @Param limit query string false "page size"
// @Param nextToken query string false "cursor"
// @Success 200 {object} object
// @Failure 400 {object} object
// @Router /api/skilifts [get]
func (h *Handler) ListAll(c *gin.Context) {
	lq, details := schemas.ParseListQuery(c.Request.URL.Query())
	if len(details) > 0 {
		apperrors.Validation(c, "invalid query", details)
		return
	}
	start, ok := decodeToken(c, lq)
	if !ok {
		return
	}
	page, serr := h.svc.ListAll(c.Request.Context(), lq.Limit, start)
	h.collection(c, page, serr)
}

// ListByLift godoc
// @Summary List one lift's collection (paginated)
// @Produce json
// @Param lift path string true "lift name"
// @Param limit query string false "page size"
// @Param nextToken query string false "cursor"
// @Success 200 {object} object
// @Failure 400 {object} object
// @Router /api/skilifts/{lift} [get]
func (h *Handler) ListByLift(c *gin.Context) {
	lift := strings.TrimSpace(c.Param("lift"))
	if lift == "" {
		apperrors.Validation(c, "invalid lift",
			[]apperrors.Detail{{Field: "Lift", Message: "must be a non-empty string"}})
		return
	}
	lq, details := schemas.ParseListQuery(c.Request.URL.Query())
	if len(details) > 0 {
		apperrors.Validation(c, "invalid query", details)
		return
	}
	start, ok := decodeToken(c, lq)
	if !ok {
		return
	}
	page, serr := h.svc.ListLift(c.Request.Context(), lift, lq.Limit, start)
	h.collection(c, page, serr)
}

// ListByRiders godoc
// @Summary List one lift's records ordered by riders desc (paginated)
// @Produce json
// @Param lift path string true "lift name"
// @Param limit query string false "page size"
// @Param nextToken query string false "cursor"
// @Param minRiders query string false "lower riders bound"
// @Param maxRiders query string false "upper riders bound"
// @Success 200 {object} object
// @Failure 400 {object} object
// @Router /api/skilifts/{lift}/by-riders [get]
func (h *Handler) ListByRiders(c *gin.Context) {
	lift := strings.TrimSpace(c.Param("lift"))
	if lift == "" {
		apperrors.Validation(c, "invalid lift",
			[]apperrors.Detail{{Field: "Lift", Message: "must be a non-empty string"}})
		return
	}
	rq, details := schemas.ParseRidersQuery(c.Request.URL.Query())
	if len(details) > 0 {
		apperrors.Validation(c, "invalid query", details)
		return
	}
	start, ok := decodeToken(c, rq.ListQuery)
	if !ok {
		return
	}
	page, serr := h.svc.ListByRiders(
		c.Request.Context(), lift, rq.Min, rq.Max, rq.Limit, start)
	h.collection(c, page, serr)
}

func decodeToken(c *gin.Context, lq schemas.ListQuery) (map[string]string, bool) {
	if !lq.HasToken {
		return nil, true
	}
	key, err := pagination.Decode(lq.Token)
	if err != nil {
		apperrors.Validation(c, "invalid query",
			[]apperrors.Detail{{Field: "nextToken", Message: "malformed token"}})
		return nil, false
	}
	return key, true
}

// metaKind classifies a :metadata path value.
func metaKind(meta string) (schemas.Kind, bool) {
	switch meta {
	case "static":
		return schemas.KindStatic, true
	case "resort":
		return schemas.KindResort, true
	default:
		if len(meta) == 8 && meta[2] == '/' && meta[5] == '/' {
			return schemas.KindDynamic, true
		}
		return "", false
	}
}

// GetOne godoc
// @Summary Get a single record
// @Produce json
// @Param lift path string true "lift name"
// @Param metadata path string true "static, resort, or MM/DD/YY"
// @Success 200 {object} object
// @Failure 400 {object} object
// @Failure 404 {object} object
// @Router /api/skilifts/{lift}/{metadata} [get]
func (h *Handler) GetOne(c *gin.Context) {
	lift := strings.TrimSpace(c.Param("lift"))
	meta := strings.TrimSpace(c.Param("metadata"))
	if lift == "" {
		apperrors.Validation(c, "invalid lift",
			[]apperrors.Detail{{Field: "Lift", Message: "must be a non-empty string"}})
		return
	}
	if _, ok := metaKind(meta); !ok {
		apperrors.Validation(c, "invalid metadata",
			[]apperrors.Detail{{Field: "Metadata", Message: `must match pattern ^static$|^resort$|^\d{2}\/\d{2}\/\d{2}$`}})
		return
	}
	item, serr := h.svc.Get(c.Request.Context(), lift, meta)
	if serr != nil {
		writeServiceError(c, serr)
		return
	}
	c.JSON(http.StatusOK, item)
}

// UpdateStatic godoc
// @Summary Partially update a static record
// @Accept json
// @Produce json
// @Param lift path string true "lift name"
// @Param body body object true "fields to update"
// @Success 200 {object} object
// @Failure 400 {object} object
// @Failure 404 {object} object
// @Router /api/skilifts/{lift}/static [put]
func (h *Handler) UpdateStatic(c *gin.Context) {
	h.update(c, strings.TrimSpace(c.Param("lift")), "static", schemas.KindStatic)
}

// UpdateOne godoc
// @Summary Partially update a dynamic or resort record
// @Accept json
// @Produce json
// @Param lift path string true "lift name"
// @Param metadata path string true "resort or MM/DD/YY"
// @Param body body object true "fields to update"
// @Success 200 {object} object
// @Failure 400 {object} object
// @Failure 404 {object} object
// @Router /api/skilifts/{lift}/{metadata} [put]
func (h *Handler) UpdateOne(c *gin.Context) {
	lift := strings.TrimSpace(c.Param("lift"))
	meta := strings.TrimSpace(c.Param("metadata"))
	kind, ok := metaKind(meta)
	if !ok || kind == schemas.KindStatic {
		apperrors.Validation(c, "invalid metadata",
			[]apperrors.Detail{{Field: "Metadata", Message: "must match path: use /static route for static records"}})
		return
	}
	h.update(c, lift, meta, kind)
}

func (h *Handler) update(c *gin.Context, lift, meta string, kind schemas.Kind) {
	if lift == "" {
		apperrors.Validation(c, "invalid lift",
			[]apperrors.Detail{{Field: "Lift", Message: "must be a non-empty string"}})
		return
	}
	raw, ok := readBody(c)
	if !ok {
		return
	}
	if details := schemas.CheckIdentity(lift, meta, raw); len(details) > 0 {
		apperrors.Validation(c, "path/body mismatch", details)
		return
	}
	updates, details := schemas.DecodeUpdate(kind, raw)
	if len(details) > 0 {
		apperrors.Validation(c, "invalid update", details)
		return
	}
	updated, serr := h.svc.Update(c.Request.Context(), lift, meta, updates)
	if serr != nil {
		writeServiceError(c, serr)
		return
	}
	c.JSON(http.StatusOK, updated)
}

// DeleteOne godoc
// @Summary Delete a record
// @Param lift path string true "lift name"
// @Param metadata path string true "static, resort, or MM/DD/YY"
// @Success 204
// @Failure 400 {object} object
// @Failure 404 {object} object
// @Router /api/skilifts/{lift}/{metadata} [delete]
func (h *Handler) DeleteOne(c *gin.Context) {
	lift := strings.TrimSpace(c.Param("lift"))
	meta := strings.TrimSpace(c.Param("metadata"))
	if lift == "" {
		apperrors.Validation(c, "invalid lift",
			[]apperrors.Detail{{Field: "Lift", Message: "must be a non-empty string"}})
		return
	}
	if _, ok := metaKind(meta); !ok {
		apperrors.Validation(c, "invalid metadata",
			[]apperrors.Detail{{Field: "Metadata", Message: `must match pattern ^static$|^resort$|^\d{2}\/\d{2}\/\d{2}$`}})
		return
	}
	if serr := h.svc.Delete(c.Request.Context(), lift, meta); serr != nil {
		writeServiceError(c, serr)
		return
	}
	c.Status(http.StatusNoContent)
}

// Health godoc
// @Summary Liveness probe
// @Produce json
// @Success 200 {object} object
// @Router /health [get]
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"uptime":    time.Since(h.started).Seconds(),
	})
}

// Ready godoc
// @Summary Readiness probe (DynamoDB)
// @Produce json
// @Success 200 {object} object
// @Failure 503 {object} object
// @Router /ready [get]
func (h *Handler) Ready(c *gin.Context) {
	if err := h.svc.Probe(c.Request.Context()); err != nil {
		apperrors.Write(c, apperrors.CodeNotReady,
			http.StatusServiceUnavailable, "service not ready", nil)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"status":    "ok",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// Root godoc
// @Summary Service info
// @Produce json
// @Success 200 {object} object
// @Router / [get]
func (h *Handler) Root(c *gin.Context) {
	region := h.cfg.Region
	if h.cfg.DynamoMode == "local" && h.cfg.LocalRegion != "" {
		region = h.cfg.LocalRegion
	}
	c.JSON(http.StatusOK, gin.H{
		"service": "gin-ddb",
		"version": "1.0.0",
		"status":  "ok",
		"table":   h.cfg.TableName,
		"region":  region,
	})
}

// Docs serves the Swagger UI.
func (h *Handler) Docs() gin.HandlerFunc {
	return ginSwagger.WrapHandler(swaggerFiles.Handler)
}

// RegisterRoutes binds the §4 table. Static segments register before
// /:lift/:metadata so they are never captured as :metadata.
func RegisterRoutes(e *gin.Engine, h *Handler) {
	e.GET("/health", h.Health)
	e.GET("/ready", h.Ready)
	e.GET("/", h.Root)
	e.GET("/docs/*any", h.Docs())

	api := e.Group("/api/skilifts")
	api.POST("/static", h.CreateStatic)
	api.POST("/dynamic", h.CreateDynamic)
	api.POST("/resort", h.CreateResort)
	api.GET("", h.ListAll)
	api.GET("/:lift/by-riders", h.ListByRiders)
	api.PUT("/:lift/static", h.UpdateStatic)
	api.GET("/:lift", h.ListByLift)
	api.GET("/:lift/:metadata", h.GetOne)
	api.PUT("/:lift/:metadata", h.UpdateOne)
	api.DELETE("/:lift/:metadata", h.DeleteOne)

	e.NoRoute(func(c *gin.Context) {
		apperrors.Write(c, apperrors.CodeNotFound,
			http.StatusNotFound, "route not found", nil)
	})
	e.NoMethod(func(c *gin.Context) {
		apperrors.Write(c, apperrors.CodeNotFound,
			http.StatusNotFound, "route not found", nil)
	})
}
