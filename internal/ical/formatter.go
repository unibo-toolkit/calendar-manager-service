package ical

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func FormatEventTitle(s string) string {
	if s == "" {
		return s
	}

	lower := strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(lower))

	capitalize := true
	for i := 0; i < len(lower); {
		r, size := utf8.DecodeRuneInString(lower[i:])

		if capitalize && unicode.IsLetter(r) {
			b.WriteRune(unicode.ToUpper(r))
			capitalize = false
		} else {
			b.WriteRune(r)
		}

		if r == '.' || r == '(' || r == '/' {
			capitalize = true
		}

		i += size
	}

	return b.String()
}
