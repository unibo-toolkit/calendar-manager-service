package ical

import (
	"embed"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed locales/*.yaml
var localesFS embed.FS

type Translations map[string]map[string]string

func LoadTranslations() Translations {
	t := make(Translations)
	entries, _ := localesFS.ReadDir("locales")
	for _, e := range entries {
		lang := strings.TrimSuffix(e.Name(), ".yaml")
		data, _ := localesFS.ReadFile("locales/" + e.Name())
		m := make(map[string]string)
		yaml.Unmarshal(data, &m)
		t[lang] = m
	}
	return t
}

func (t Translations) Get(lang, key string) string {
	if m, ok := t[lang]; ok {
		if v, ok := m[key]; ok {
			return v
		}
	}
	if m, ok := t["it"]; ok {
		return m[key]
	}
	return key
}
