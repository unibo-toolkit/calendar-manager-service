package calendar

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/bwmarrin/snowflake"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/unibo-toolkit/calendar-manager-service/internal/config"
	"github.com/unibo-toolkit/calendar-manager-service/internal/ical"
	"github.com/unibo-toolkit/calendar-manager-service/internal/storage"
	"github.com/unibo-toolkit/calendar-manager-service/internal/storage/db"
)

type Service struct {
	log       *slog.Logger
	cfg       *config.Config
	storage   *storage.Storage
	scraper   *ScraperClient
	snowflake *snowflake.Node
	ical      *ical.Generator
}

type CalendarResponse struct {
	ID           string               `json:"id"`
	Slug         string               `json:"slug"`
	Name         string               `json:"name"`
	Description  *string              `json:"description"`
	Lang         string               `json:"lang"`
	IcsURL       string               `json:"ics_url"`
	TTLExpiresAt time.Time            `json:"ttl_expires_at"`
	OwnerID      *string              `json:"owner_id"`
	Courses      []CourseResponseItem `json:"courses"`
	CreatedAt    time.Time            `json:"created_at"`
}

type CourseResponseItem struct {
	ID         string             `json:"id"`
	Curriculum CurriculumResponse `json:"curriculum"`
	Subjects   []SubjectResponse  `json:"subjects"`
}

type CurriculumResponse struct {
	ID           string         `json:"id"`
	Code         string         `json:"code"`
	AcademicYear int16          `json:"academic_year"`
	Label        string         `json:"label"`
	Course       CourseResponse `json:"course"`
}

type CourseResponse struct {
	ID      string  `json:"id"`
	TitleIt string  `json:"title_it"`
	TitleEn *string `json:"title_en"`
}

type SubjectResponse struct {
	ID         string  `json:"id"`
	Title      string  `json:"title"`
	ModuleCode *string `json:"module_code"`
	Credits    *int16  `json:"credits"`
	Professor  *string `json:"professor"`
}

type CalendarListResponse struct {
	Items []CalendarListItem `json:"items"`
}

type CalendarListItem struct {
	ID             string     `json:"id"`
	Slug           string     `json:"slug"`
	Name           string     `json:"name"`
	IcsURL         string     `json:"ics_url"`
	AccessCount    int32      `json:"access_count"`
	LastAccessedAt *time.Time `json:"last_accessed_at"`
	TTLExpiresAt   time.Time  `json:"ttl_expires_at"`
	CoursesCount   int64      `json:"courses_count"`
	CreatedAt      time.Time  `json:"created_at"`
}

type ScraperClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewScraperClient(baseURL string, timeoutSec int) *ScraperClient {
	return &ScraperClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: time.Duration(timeoutSec) * time.Second},
	}
}

func (c *ScraperClient) RefreshCurriculum(ctx context.Context, curriculumID string) error {
	url := fmt.Sprintf("%s/api/v1/timetable/refresh?curriculum_id=%s", c.baseURL, curriculumID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrScraperUnavailable, err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrScraperUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("%w: status %d", ErrScraperUnavailable, resp.StatusCode)
	}
	return nil
}

func NewService(log *slog.Logger, cfg *config.Config, st *storage.Storage) *Service {
	node, err := snowflake.NewNode(cfg.Snowflake.NodeID)
	if err != nil {
		panic("failed to create snowflake node: " + err.Error())
	}
	scraperClient := NewScraperClient(cfg.Scraper.BaseURL, cfg.Scraper.Timeout)
	icalGen := ical.NewGenerator(cfg.Calendar.ProdID, cfg.Calendar.Timezone, cfg.Calendar.Domain)

	return &Service{
		log: log, cfg: cfg, storage: st,
		scraper: scraperClient, snowflake: node, ical: icalGen,
	}
}

func (s *Service) CreateCalendar(ctx context.Context, userID string, req CreateCalendarRequest) (*CalendarResponse, error) {
	log := s.log.With("op", "CreateCalendar", "name", req.Name, "user_id", userID, "lang", req.Lang, "courses_count", len(req.Courses))
	log.Info("creating calendar")

	slug := s.snowflake.Generate().String()

	var ownerID pgtype.UUID
	var ttlExpiresAt time.Time
	if userID != "" {
		uid, err := uuid.Parse(userID)
		if err != nil {
			return nil, ErrInvalidInput
		}
		ownerID = pgtype.UUID{Bytes: uid, Valid: true}
		ttlExpiresAt = time.Now().Add(time.Duration(s.cfg.TTL.AuthenticatedDays) * 24 * time.Hour)
	} else {
		ttlExpiresAt = time.Now().Add(time.Duration(s.cfg.TTL.AnonymousDays) * 24 * time.Hour)
	}

	tx, err := s.storage.Pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	qtx := s.storage.Queries.WithTx(tx)

	cal, err := qtx.CreateCalendarLink(ctx, db.CreateCalendarLinkParams{
		Slug:         slug,
		OwnerID:      ownerID,
		Name:         req.Name,
		Lang:         req.Lang,
		TtlExpiresAt: pgtype.Timestamptz{Time: ttlExpiresAt, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("create calendar link: %w", err)
	}

	if err := s.validateAndInsertCourses(ctx, qtx, cal.ID, req.Courses); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}

	s.refreshCurricula(ctx, log, req.Courses)

	resp, err := s.buildCalendarResponse(ctx, cal)
	if err != nil {
		return nil, err
	}

	log.Info("calendar created", "id", pgtypeUUIDToString(cal.ID), "slug", slug)
	return resp, nil
}

func (s *Service) GetCalendarICS(ctx context.Context, slug, userAgent, ipAddress string) ([]byte, error) {
	log := s.log.With("op", "GetCalendarICS", "slug", slug)
	log.Info("generating ics")

	cal, err := s.storage.GetCalendarBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCalendarNotFound
		}
		return nil, fmt.Errorf("get calendar by slug: %w", err)
	}

	if cal.TtlExpiresAt.Valid && time.Now().After(cal.TtlExpiresAt.Time) {
		return nil, ErrCalendarExpired
	}

	s.refreshTTL(ctx, &cal)

	go func() {
		bgCtx := context.Background()
		if err := s.storage.IncrementAccess(bgCtx, cal.ID); err != nil {
			log.Warn("increment access failed", "error", err)
		}
		ipHash := fmt.Sprintf("%x", sha256.Sum256([]byte(ipAddress)))
		if err := s.storage.UpsertSubscription(bgCtx, db.UpsertSubscriptionParams{
			CalendarID: cal.ID,
			UserAgent:  pgtype.Text{String: userAgent, Valid: true},
			IpHash:     pgtype.Text{String: ipHash, Valid: true},
		}); err != nil {
			log.Warn("upsert subscription failed", "error", err)
		}
	}()

	events, err := s.storage.GetEventsForCalendar(ctx, cal.ID)
	if err != nil {
		return nil, fmt.Errorf("get events: %w", err)
	}

	eventData := make([]ical.EventData, len(events))
	for i, e := range events {
		eventData[i] = ical.EventData{
			TimetableEventID: pgtypeUUIDToString(e.TimetableEventID),
			Title:            e.Title,
			StartDatetime:    e.StartDatetime.Time,
			EndDatetime:      e.EndDatetime.Time,
			Professor:        pgtypeTextToPtr(e.Professor),
			ModuleCode:       pgtypeTextToPtr(e.ModuleCode),
			Credits:          e.Credits,
			IsRemote:         e.IsRemote,
			TeamsLink:        pgtypeTextToPtr(e.TeamsLink),
			Notes:            pgtypeTextToPtr(e.Notes),
			GroupID:          pgtypeTextToPtr(e.GroupID),
			Sequence:         e.Sequence,
			ClassroomName:    pgtypeTextToPtr(e.ClassroomName),
			ClassroomAddress: pgtypeTextToPtr(e.ClassroomAddress),
			Latitude:         e.Latitude,
			Longitude:        e.Longitude,
		}
	}

	data, err := s.ical.Generate(cal.Name, slug, eventData, cal.Lang)
	if err != nil {
		return nil, fmt.Errorf("generate ics: %w", err)
	}

	log.Info("ics generated", "events_count", len(events))
	return data, nil
}

func (s *Service) ListCalendars(ctx context.Context, userID string) (*CalendarListResponse, error) {
	log := s.log.With("op", "ListCalendars", "user_id", userID)
	log.Info("listing calendars")

	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, ErrInvalidInput
	}

	calendars, err := s.storage.ListCalendarsByOwner(ctx, pgtype.UUID{Bytes: uid, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("list calendars: %w", err)
	}

	items := make([]CalendarListItem, len(calendars))
	for i, c := range calendars {
		var lastAccessed *time.Time
		if c.LastAccessedAt.Valid {
			lastAccessed = &c.LastAccessedAt.Time
		}
		items[i] = CalendarListItem{
			ID:             pgtypeUUIDToString(c.ID),
			Slug:           c.Slug,
			Name:           c.Name,
			IcsURL:         s.buildIcsURL(c.Slug),
			AccessCount:    c.AccessCount,
			LastAccessedAt: lastAccessed,
			TTLExpiresAt:   c.TtlExpiresAt.Time,
			CoursesCount:   c.CoursesCount,
			CreatedAt:      c.CreatedAt.Time,
		}
	}

	log.Info("calendars listed", "count", len(items))
	return &CalendarListResponse{Items: items}, nil
}

func (s *Service) GetCalendar(ctx context.Context, userID, calendarID string) (*CalendarResponse, error) {
	s.log.With("op", "GetCalendar", "calendar_id", calendarID, "user_id", userID).Info("getting calendar")

	calPgID := uuidToPgtype(calendarID)

	cal, err := s.storage.GetCalendarByID(ctx, calPgID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrCalendarNotFound
		}
		return nil, fmt.Errorf("get calendar: %w", err)
	}

	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, ErrInvalidInput
	}
	if !cal.OwnerID.Valid || cal.OwnerID.Bytes != uid {
		return nil, ErrNotOwner
	}

	return s.buildCalendarResponse(ctx, cal)
}

func (s *Service) UpdateCalendar(ctx context.Context, userID, calendarID string, req UpdateCalendarRequest) (*CalendarResponse, error) {
	log := s.log.With("op", "UpdateCalendar", "calendar_id", calendarID, "user_id", userID)
	log.Info("updating calendar")

	calPgID := uuidToPgtype(calendarID)

	cal, err := s.storage.GetCalendarByID(ctx, calPgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCalendarNotFound
		}
		return nil, fmt.Errorf("get calendar: %w", err)
	}

	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, ErrInvalidInput
	}
	if !cal.OwnerID.Valid || cal.OwnerID.Bytes != uid {
		return nil, ErrNotOwner
	}

	var name pgtype.Text
	if req.Name != nil {
		name = pgtype.Text{String: *req.Name, Valid: true}
	}
	var desc pgtype.Text
	if req.Description != nil {
		desc = pgtype.Text{String: *req.Description, Valid: true}
	}
	var lang pgtype.Text
	if req.Lang != nil {
		lang = pgtype.Text{String: *req.Lang, Valid: true}
	}

	cal, err = s.storage.UpdateCalendarFields(ctx, db.UpdateCalendarFieldsParams{
		ID:          calPgID,
		Name:        name,
		Description: desc,
		Lang:        lang,
	})
	if err != nil {
		return nil, fmt.Errorf("update calendar fields: %w", err)
	}

	if req.Courses != nil {
		tx, err := s.storage.Pool.Begin(ctx)
		if err != nil {
			return nil, fmt.Errorf("begin tx: %w", err)
		}
		defer tx.Rollback(ctx)

		qtx := s.storage.Queries.WithTx(tx)

		if err := qtx.DeleteCalendarCoursesByCalendar(ctx, calPgID); err != nil {
			return nil, fmt.Errorf("delete courses: %w", err)
		}

		if err := s.validateAndInsertCourses(ctx, qtx, calPgID, req.Courses); err != nil {
			return nil, err
		}

		if err := tx.Commit(ctx); err != nil {
			return nil, fmt.Errorf("commit tx: %w", err)
		}

		s.refreshCurricula(ctx, log, req.Courses)
	}

	cal, err = s.storage.GetCalendarByID(ctx, calPgID)
	if err != nil {
		return nil, fmt.Errorf("get updated calendar: %w", err)
	}

	log.Info("calendar updated")
	return s.buildCalendarResponse(ctx, cal)
}

func (s *Service) DeleteCalendar(ctx context.Context, userID, calendarID string) error {
	log := s.log.With("op", "DeleteCalendar", "calendar_id", calendarID, "user_id", userID)
	log.Info("deleting calendar")

	calPgID := uuidToPgtype(calendarID)

	cal, err := s.storage.GetCalendarByID(ctx, calPgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrCalendarNotFound
		}
		return fmt.Errorf("get calendar: %w", err)
	}

	uid, err := uuid.Parse(userID)
	if err != nil {
		return ErrInvalidInput
	}
	if !cal.OwnerID.Valid || cal.OwnerID.Bytes != uid {
		return ErrNotOwner
	}

	if err := s.storage.DeleteCalendar(ctx, calPgID); err != nil {
		return fmt.Errorf("delete calendar: %w", err)
	}

	log.Info("calendar deleted")
	return nil
}

func (s *Service) ClaimCalendar(ctx context.Context, userID, calendarID string) (*CalendarResponse, error) {
	log := s.log.With("op", "ClaimCalendar", "calendar_id", calendarID, "user_id", userID)
	log.Info("claiming calendar")

	calPgID := uuidToPgtype(calendarID)

	cal, err := s.storage.GetCalendarByID(ctx, calPgID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCalendarNotFound
		}
		return nil, fmt.Errorf("get calendar: %w", err)
	}

	if cal.OwnerID.Valid {
		return nil, ErrAlreadyClaimed
	}

	uid, err := uuid.Parse(userID)
	if err != nil {
		return nil, ErrInvalidInput
	}

	newTTL := time.Now().Add(time.Duration(s.cfg.TTL.AuthenticatedDays) * 24 * time.Hour)
	claimed, err := s.storage.ClaimCalendar(ctx, db.ClaimCalendarParams{
		ID:           calPgID,
		OwnerID:      pgtype.UUID{Bytes: uid, Valid: true},
		TtlExpiresAt: pgtype.Timestamptz{Time: newTTL, Valid: true},
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAlreadyClaimed
		}
		return nil, fmt.Errorf("claim calendar: %w", err)
	}

	log.Info("calendar claimed")
	return s.buildCalendarResponse(ctx, claimed)
}

type PublicCalendarResponse struct {
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	Slug         string               `json:"slug"`
	Lang         string               `json:"lang"`
	Courses      []CourseResponseItem `json:"courses"`
	Claimed      bool                 `json:"claimed"`
	TotalEvents  int32                `json:"total_events"`
	TTLExpiresAt time.Time            `json:"ttl_expires_at"`
	CreatedAt    time.Time            `json:"created_at"`
}

func (s *Service) GetPublicCalendar(ctx context.Context, slug string) (*PublicCalendarResponse, error) {
	log := s.log.With("op", "GetPublicCalendar", "slug", slug)
	log.Info("getting public calendar")

	cal, err := s.storage.GetCalendarBySlug(ctx, slug)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCalendarNotFound
		}
		return nil, fmt.Errorf("get calendar by slug: %w", err)
	}

	if cal.TtlExpiresAt.Valid && time.Now().After(cal.TtlExpiresAt.Time) {
		return nil, ErrCalendarExpired
	}

	totalEvents, err := s.storage.CountEventsForCalendar(ctx, cal.ID)
	if err != nil {
		return nil, fmt.Errorf("count events: %w", err)
	}

	resp, err := s.buildCalendarResponse(ctx, cal)
	if err != nil {
		return nil, err
	}

	log.Info("public calendar retrieved", "total_events", totalEvents)
	return &PublicCalendarResponse{
		ID:           resp.ID,
		Name:         resp.Name,
		Slug:         resp.Slug,
		Lang:         resp.Lang,
		Courses:      resp.Courses,
		Claimed:      cal.OwnerID.Valid && cal.OwnerID.Bytes != uuid.Nil,
		TotalEvents:  totalEvents,
		TTLExpiresAt: cal.TtlExpiresAt.Time,
		CreatedAt:    cal.CreatedAt.Time,
	}, nil
}

func (s *Service) validateAndInsertCourses(ctx context.Context, qtx *db.Queries, calendarID pgtype.UUID, courses []CourseInputItem) error {
	for i, course := range courses {
		currID := uuidToPgtype(course.CurriculumID)
		_, err := qtx.CheckCurriculumExists(ctx, currID)
		if err != nil {
			return ErrCurriculumNotFound
		}
		for _, subID := range course.SubjectIDs {
			_, err := qtx.CheckSubjectBelongsToCurriculum(ctx, db.CheckSubjectBelongsToCurriculumParams{
				ID:           uuidToPgtype(subID),
				CurriculumID: currID,
			})
			if err != nil {
				return ErrSubjectWrongCurriculum
			}
		}

		cc, err := qtx.CreateCalendarCourse(ctx, db.CreateCalendarCourseParams{
			CalendarID:   calendarID,
			CurriculumID: currID,
			Position:     int16(i),
		})
		if err != nil {
			return fmt.Errorf("create calendar course: %w", err)
		}
		for _, subID := range course.SubjectIDs {
			if err := qtx.CreateCalendarSubject(ctx, db.CreateCalendarSubjectParams{
				CalendarCourseID: cc.ID,
				SubjectID:        uuidToPgtype(subID),
			}); err != nil {
				return fmt.Errorf("create calendar subject: %w", err)
			}
		}
	}
	return nil
}

func (s *Service) refreshCurricula(ctx context.Context, log *slog.Logger, courses []CourseInputItem) {
	uniqueCurriculums := make(map[string]bool)
	for _, course := range courses {
		uniqueCurriculums[course.CurriculumID] = true
	}
	for currID := range uniqueCurriculums {
		if err := s.scraper.RefreshCurriculum(ctx, currID); err != nil {
			log.Warn("scraper refresh failed", "curriculum_id", currID, "error", err)
		}
	}
}

func (s *Service) refreshTTL(ctx context.Context, cal *db.CalendarLink) {
	if !cal.TtlExpiresAt.Valid {
		return
	}
	window := time.Duration(s.cfg.TTL.RefreshWindowDays) * 24 * time.Hour
	if time.Until(cal.TtlExpiresAt.Time) < window {
		var days int
		if cal.OwnerID.Valid {
			days = s.cfg.TTL.AuthenticatedDays
		} else {
			days = s.cfg.TTL.AnonymousDays
		}
		extension := time.Duration(days) * 24 * time.Hour
		_ = s.storage.ExtendTTL(ctx, db.ExtendTTLParams{
			ID:           cal.ID,
			TtlExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(extension), Valid: true},
		})
	}
}

func (s *Service) buildCalendarResponse(ctx context.Context, cal db.CalendarLink) (*CalendarResponse, error) {
	courses, err := s.storage.GetCalendarCoursesWithDetails(ctx, cal.ID)
	if err != nil {
		return nil, fmt.Errorf("get courses with details: %w", err)
	}

	courseIDs := make([]pgtype.UUID, len(courses))
	for i, c := range courses {
		courseIDs[i] = c.CalendarCourseID
	}

	var subjects []db.GetCalendarSubjectsRow
	if len(courseIDs) > 0 {
		subjects, err = s.storage.GetCalendarSubjects(ctx, courseIDs)
		if err != nil {
			return nil, fmt.Errorf("get subjects: %w", err)
		}
	}

	subjectsByCC := make(map[[16]byte][]SubjectResponse)
	for _, sub := range subjects {
		key := sub.CalendarCourseID.Bytes
		var moduleCode *string
		if sub.ModuleCode.Valid {
			moduleCode = &sub.ModuleCode.String
		}
		var credits *int16
		if sub.Credits.Valid {
			credits = &sub.Credits.Int16
		}
		var professor *string
		if sub.Professor.Valid {
			professor = &sub.Professor.String
		}
		subjectsByCC[key] = append(subjectsByCC[key], SubjectResponse{
			ID:         pgtypeUUIDToString(sub.SubjectID),
			Title:      sub.Title,
			ModuleCode: moduleCode,
			Credits:    credits,
			Professor:  professor,
		})
	}

	courseItems := make([]CourseResponseItem, len(courses))
	for i, c := range courses {
		subs := subjectsByCC[c.CalendarCourseID.Bytes]
		if subs == nil {
			subs = []SubjectResponse{}
		}
		var titleEn *string
		if c.CourseTitleEn.Valid {
			titleEn = &c.CourseTitleEn.String
		}
		courseItems[i] = CourseResponseItem{
			ID: pgtypeUUIDToString(c.CalendarCourseID),
			Curriculum: CurriculumResponse{
				ID:           pgtypeUUIDToString(c.CurriculumID),
				Code:         c.CurriculumCode,
				AcademicYear: c.AcademicYear,
				Label:        c.CurriculumLabel,
				Course: CourseResponse{
					ID:      pgtypeUUIDToString(c.CourseID),
					TitleIt: c.CourseTitleIt,
					TitleEn: titleEn,
				},
			},
			Subjects: subs,
		}
	}

	var ownerIDStr *string
	if cal.OwnerID.Valid {
		s := pgtypeUUIDToString(cal.OwnerID)
		ownerIDStr = &s
	}

	var description *string
	if cal.Description.Valid {
		description = &cal.Description.String
	}

	return &CalendarResponse{
		ID:           pgtypeUUIDToString(cal.ID),
		Slug:         cal.Slug,
		Name:         cal.Name,
		Description:  description,
		Lang:         cal.Lang,
		IcsURL:       s.buildIcsURL(cal.Slug),
		TTLExpiresAt: cal.TtlExpiresAt.Time,
		OwnerID:      ownerIDStr,
		Courses:      courseItems,
		CreatedAt:    cal.CreatedAt.Time,
	}, nil
}

func (s *Service) buildIcsURL(slug string) string {
	return s.cfg.Calendar.BaseURL + "/cal/" + slug + ".ics"
}

func uuidToPgtype(s string) pgtype.UUID {
	uid, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{}
	}
	return pgtype.UUID{Bytes: uid, Valid: true}
}

func pgtypeUUIDToString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	return uuid.UUID(u.Bytes).String()
}

func pgtypeTextToPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}
