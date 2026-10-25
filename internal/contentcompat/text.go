package contentcompat

import (
	"sort"
	"strings"
)

// LocalizedText is the backend-owned chrome map used to keep /go-json/go/v2
// multi-locale fields while Codex v0.4 stores one string per record.
type LocalizedText map[string]string

// Value returns the text for locale, then fallback, then the first sorted key.
func (text LocalizedText) Value(locale, fallback string) string {
	if text == nil {
		return ""
	}
	if locale != "" {
		if value := strings.TrimSpace(text[locale]); value != "" {
			return text[locale]
		}
	}
	if fallback != "" {
		if value := strings.TrimSpace(text[fallback]); value != "" {
			return text[fallback]
		}
	}
	keys := make([]string, 0, len(text))
	for key := range text {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if strings.TrimSpace(text[key]) != "" {
			return text[key]
		}
	}
	if len(keys) == 0 {
		return ""
	}
	return text[keys[0]]
}

// Merge copies patch over target. Empty patch leaves target unchanged.
func (text LocalizedText) Merge(patch LocalizedText) LocalizedText {
	if len(patch) == 0 {
		return text
	}
	if text == nil {
		text = LocalizedText{}
	}
	for locale, value := range patch {
		text[locale] = value
	}
	return text
}

func (text LocalizedText) clone() LocalizedText {
	if text == nil {
		return nil
	}
	cloned := make(LocalizedText, len(text))
	for locale, value := range text {
		cloned[locale] = value
	}
	return cloned
}

func (text LocalizedText) Empty() bool {
	if len(text) == 0 {
		return true
	}
	for _, value := range text {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

func single(locale, value string) LocalizedText {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	if locale == "" {
		locale = "en"
	}
	return LocalizedText{locale: value}
}
