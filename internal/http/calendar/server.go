package calendar

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/unibo-toolkit/calendar-manager-service/internal/config"
	"github.com/unibo-toolkit/calendar-manager-service/internal/storage"
)

type Server struct {
	log     *slog.Logger
	service *Service
	cfg     *config.Config
	*gin.Engine
	*http.Server
}

type CreateCalendarRequest struct {
	Name    string            `json:"name" binding:"required,min=1,max=255"`
	Lang    string            `json:"lang"`
	Courses []CourseInputItem `json:"courses" binding:"required,min=1,dive"`
}

type CourseInputItem struct {
	CurriculumID string   `json:"curriculum_id" binding:"required,uuid"`
	SubjectIDs   []string `json:"subject_ids" binding:"required,min=1,dive,uuid"`
}

type UpdateCalendarRequest struct {
	Name        *string           `json:"name,omitempty"`
	Description *string           `json:"description,omitempty"`
	Lang        *string           `json:"lang,omitempty"`
	Courses     []CourseInputItem `json:"courses,omitempty"`
}

func New(log *slog.Logger, cfg *config.Config, st *storage.Storage) *Server {
	if cfg.HTTP.Environment == "prod" {
		gin.SetMode(gin.ReleaseMode)
	}

	engine := gin.Default()
	if cfg.HTTP.IPHeader != "" {
		engine.RemoteIPHeaders = []string{cfg.HTTP.IPHeader}
	}
	service := NewService(log, cfg, st)
	srv := &Server{log: log, service: service, cfg: cfg, Engine: engine}
	srv.registerRoutes()
	srv.Server = &http.Server{Addr: ":" + cfg.HTTP.Port, Handler: engine.Handler()}
	return srv
}

func (s *Server) registerRoutes() {
	v1 := s.Group("/api/v1")

	calendars := v1.Group("/calendars")
	calendars.Use(s.optionalAuth)
	calendars.POST("", s.handleCreateCalendar)

	authed := calendars.Group("")
	authed.Use(s.requireAuth)
	authed.GET("", s.handleListCalendars)
	authed.GET("/:id", s.handleGetCalendar)
	authed.PATCH("/:id", s.handleUpdateCalendar)
	authed.DELETE("/:id", s.handleDeleteCalendar)
	authed.POST("/:id/claim", s.handleClaimCalendar)

	s.GET("/cal/:slug", s.handleGetICS)
}

func (s *Server) optionalAuth(c *gin.Context) {
	c.Set("user_id", c.GetHeader("X-User-Id"))
	c.Set("email", c.GetHeader("X-Email"))
	c.Set("roles", decodeRoles(c.GetHeader("X-Roles")))
	c.Next()
}

func decodeRoles(raw string) []string {
	if raw == "" {
		return nil
	}

	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(raw)
		if err != nil {
			return []string{raw}
		}
	}

	var roles []string
	if err := json.Unmarshal(decoded, &roles); err != nil {
		return []string{string(decoded)}
	}
	return roles
}

func (s *Server) requireAuth(c *gin.Context) {
	if c.GetString("user_id") == "" {
		c.AbortWithStatusJSON(401, gin.H{"error": ErrUnauthorized.Error()})
		return
	}
	c.Next()
}

func (s *Server) handleCreateCalendar(c *gin.Context) {
	var req CreateCalendarRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": ErrInvalidInput.Error()})
		return
	}

	if req.Lang == "" {
		req.Lang = "it"
	}
	if req.Lang != "it" && req.Lang != "en" {
		c.JSON(400, gin.H{"error": ErrInvalidLang.Error()})
		return
	}

	userID := c.GetString("user_id")

	result, err := s.service.CreateCalendar(c.Request.Context(), userID, req)
	if err != nil {
		s.handleError(c, err)
		return
	}

	c.JSON(201, result)
}

func (s *Server) handleGetICS(c *gin.Context) {
	slug := c.Param("slug")
	slug = strings.TrimSuffix(slug, ".ics")

	userAgent := c.GetHeader("User-Agent")
	ip := c.ClientIP()

	data, err := s.service.GetCalendarICS(c.Request.Context(), slug, userAgent, ip)
	if err != nil {
		s.handleError(c, err)
		return
	}

	c.Header("Content-Type", "text/calendar; charset=utf-8")
	c.Header("Content-Disposition", `attachment; filename="calendar.ics"`)
	c.Header("Cache-Control", "no-cache, no-store")
	c.Data(200, "text/calendar; charset=utf-8", data)
}

func (s *Server) handleListCalendars(c *gin.Context) {
	userID := c.GetString("user_id")

	result, err := s.service.ListCalendars(c.Request.Context(), userID)
	if err != nil {
		s.handleError(c, err)
		return
	}

	c.JSON(200, result)
}

func (s *Server) handleGetCalendar(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetString("user_id")

	result, err := s.service.GetCalendar(c.Request.Context(), userID, id)
	if err != nil {
		s.handleError(c, err)
		return
	}

	c.JSON(200, result)
}

func (s *Server) handleUpdateCalendar(c *gin.Context) {
	var req UpdateCalendarRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": ErrInvalidInput.Error()})
		return
	}

	if req.Lang != nil && *req.Lang != "it" && *req.Lang != "en" {
		c.JSON(400, gin.H{"error": ErrInvalidLang.Error()})
		return
	}

	id := c.Param("id")
	userID := c.GetString("user_id")

	result, err := s.service.UpdateCalendar(c.Request.Context(), userID, id, req)
	if err != nil {
		s.handleError(c, err)
		return
	}

	c.JSON(200, result)
}

func (s *Server) handleDeleteCalendar(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetString("user_id")

	err := s.service.DeleteCalendar(c.Request.Context(), userID, id)
	if err != nil {
		s.handleError(c, err)
		return
	}

	c.Status(204)
}

func (s *Server) handleClaimCalendar(c *gin.Context) {
	id := c.Param("id")
	userID := c.GetString("user_id")

	result, err := s.service.ClaimCalendar(c.Request.Context(), userID, id)
	if err != nil {
		s.handleError(c, err)
		return
	}

	c.JSON(200, result)
}

func (s *Server) handleError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrCalendarNotFound):
		c.JSON(404, gin.H{"error": err.Error()})
	case errors.Is(err, ErrCalendarExpired):
		c.JSON(410, gin.H{"error": err.Error()})
	case errors.Is(err, ErrNotOwner):
		c.JSON(403, gin.H{"error": err.Error()})
	case errors.Is(err, ErrAlreadyClaimed):
		c.JSON(409, gin.H{"error": err.Error()})
	case errors.Is(err, ErrUnauthorized):
		c.JSON(401, gin.H{"error": err.Error()})
	case errors.Is(err, ErrCurriculumNotFound),
		errors.Is(err, ErrSubjectWrongCurriculum),
		errors.Is(err, ErrInvalidInput),
		errors.Is(err, ErrInvalidLang):
		c.JSON(400, gin.H{"error": err.Error()})
	default:
		s.log.Error("internal error", "error", err)
		c.JSON(500, gin.H{"error": "internal server error"})
	}
}
