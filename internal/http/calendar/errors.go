package calendar

import "errors"

var (
	ErrCalendarNotFound       = errors.New("calendar not found")
	ErrCalendarExpired        = errors.New("calendar has expired")
	ErrCurriculumNotFound     = errors.New("curriculum not found")
	ErrSubjectWrongCurriculum = errors.New("subject does not belong to curriculum")
	ErrNotOwner               = errors.New("not the calendar owner")
	ErrAlreadyClaimed         = errors.New("calendar already claimed by another user")
	ErrUnauthorized           = errors.New("authorization required")
	ErrInvalidInput           = errors.New("invalid input data")
	ErrScraperUnavailable     = errors.New("scraper service unavailable")
	ErrInvalidLang            = errors.New("invalid language, must be 'it' or 'en'")
	ErrInvalidFamilyHash      = errors.New("invalid event family hash")
	ErrTooManyFamilies        = errors.New("too many excluded event families")
	ErrUnknownFamily          = errors.New("unknown event family hash")
)
