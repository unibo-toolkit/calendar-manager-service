package ical

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

type Generator struct {
	prodID       string
	timezone     string
	domain       string
	translations Translations
}

func NewGenerator(prodID, timezone, domain string) *Generator {
	return &Generator{
		prodID:       prodID,
		timezone:     timezone,
		domain:       domain,
		translations: LoadTranslations(),
	}
}

type EventData struct {
	TimetableEventID string
	Title            string
	StartDatetime    time.Time
	EndDatetime      time.Time
	Professor        *string
	ModuleCode       *string
	Credits          pgtype.Numeric
	IsRemote         bool
	TeamsLink        *string
	Notes            *string
	GroupID          *string
	Sequence         int32
	ClassroomName    *string
	ClassroomAddress *string
	Latitude         pgtype.Numeric
	Longitude        pgtype.Numeric
}

func (g *Generator) Generate(calendarName, slug string, events []EventData, lang string) ([]byte, error) {
	var buf bytes.Buffer

	buf.WriteString("BEGIN:VCALENDAR\r\n")
	buf.WriteString("VERSION:2.0\r\n")
	buf.WriteString(fmt.Sprintf("PRODID:%s\r\n", g.prodID))
	buf.WriteString("CALSCALE:GREGORIAN\r\n")
	buf.WriteString("METHOD:PUBLISH\r\n")
	buf.WriteString(fmt.Sprintf("X-WR-CALNAME:%s\r\n", escapeText(calendarName)))
	buf.WriteString(fmt.Sprintf("X-WR-TIMEZONE:%s\r\n", g.timezone))
	buf.WriteString("X-PUBLISHED-TTL:PT24H\r\n")
	buf.WriteString(EuropeRomeVTimezone)
	buf.WriteString("\r\n")

	for _, e := range events {
		g.writeEvent(&buf, slug, e, lang)
	}

	buf.WriteString("END:VCALENDAR\r\n")
	return buf.Bytes(), nil
}

func (g *Generator) writeEvent(buf *bytes.Buffer, slug string, e EventData, lang string) {
	now := time.Now().UTC()

	buf.WriteString("BEGIN:VEVENT\r\n")
	buf.WriteString(fmt.Sprintf("UID:%s-%s@%s\r\n", slug, e.TimetableEventID, g.domain))
	buf.WriteString(fmt.Sprintf("DTSTAMP:%s\r\n", now.Format("20060102T150405Z")))
	buf.WriteString(fmt.Sprintf("DTSTART:%s\r\n", e.StartDatetime.UTC().Format("20060102T150405Z")))
	buf.WriteString(fmt.Sprintf("DTEND:%s\r\n", e.EndDatetime.UTC().Format("20060102T150405Z")))
	buf.WriteString(fmt.Sprintf("SUMMARY:%s\r\n", escapeText(e.Title)))
	buf.WriteString(fmt.Sprintf("SEQUENCE:%d\r\n", e.Sequence))
	buf.WriteString("STATUS:CONFIRMED\r\n")

	desc := buildDescription(e, g.translations, lang)
	if desc != "" {
		buf.WriteString(fmt.Sprintf("DESCRIPTION:%s\r\n", desc))
	}

	if e.IsRemote {
		buf.WriteString(fmt.Sprintf("LOCATION:%s\r\n", g.translations.Get(lang, "online")))
		if e.TeamsLink != nil {
			buf.WriteString(fmt.Sprintf("URL:%s\r\n", *e.TeamsLink))
		}
	} else if e.ClassroomName != nil {
		location := escapeText(*e.ClassroomName)
		if e.ClassroomAddress != nil {
			location += "\\, " + escapeText(*e.ClassroomAddress)
		}
		buf.WriteString(fmt.Sprintf("LOCATION:%s\r\n", location))
		if e.Latitude.Valid && e.Longitude.Valid {
			lat, _ := numericToFloat(e.Latitude)
			lon, _ := numericToFloat(e.Longitude)
			buf.WriteString(fmt.Sprintf("GEO:%f;%f\r\n", lat, lon))
		}
	}

	buf.WriteString("END:VEVENT\r\n")
}

func buildDescription(e EventData, t Translations, lang string) string {
	var parts []string
	if e.Professor != nil {
		parts = append(parts, escapeText(t.Get(lang, "professor")+": "+*e.Professor))
	}
	if e.ModuleCode != nil {
		parts = append(parts, escapeText(t.Get(lang, "module")+": "+*e.ModuleCode))
	}
	if e.Credits.Valid {
		credits, ok := numericToFloat(e.Credits)
		if ok && credits > 0 {
			parts = append(parts, escapeText(fmt.Sprintf("%s: %g CFU", t.Get(lang, "credits"), credits)))
		}
	}
	if e.Notes != nil && strings.TrimSpace(*e.Notes) != "" {
		parts = append(parts, escapeText(t.Get(lang, "notes")+": "+*e.Notes))
	}
	return strings.Join(parts, "\\n")
}

func escapeText(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

func numericToFloat(n pgtype.Numeric) (float64, bool) {
	if !n.Valid {
		return 0, false
	}
	f, err := n.Float64Value()
	if err != nil {
		return 0, false
	}
	return f.Float64, true
}
