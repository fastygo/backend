// Package contentcompat keeps GoBackend storage migrations that are not part of Codex.
package contentcompat

import (
	"strings"

	"github.com/fastygo/codex/content"
)

// Lift moves legacy payload_<locale> metadata into locale documents.
func Lift(entry *content.Entry) {
	if entry == nil {
		return
	}
	if entry.Locales == nil {
		entry.Locales = map[string]content.LocaleDocument{}
	}
	if entry.Metadata == nil {
		return
	}
	for key, value := range entry.Metadata {
		locale, ok := strings.CutPrefix(key, "payload_")
		if !ok {
			continue
		}
		locale = content.NormalizeLocale(locale)
		if locale == "" {
			continue
		}
		document, _ := value.Value.(map[string]any)
		if document == nil {
			continue
		}
		if _, exists := entry.Locales[locale]; !exists {
			entry.Locales[locale] = content.LocaleDocument{
				Data:   document,
				Status: entry.Status,
			}
		}
		delete(entry.Metadata, key)
	}
}
