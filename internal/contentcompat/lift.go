package contentcompat

import (
	"strings"

	"github.com/fastygo/codex/content"
)

// Lift moves legacy payload_<locale> metadata into FormSet locale documents.
func Lift(entry *content.Entry) {
	if entry == nil {
		return
	}
	documents := Documents(*entry)
	if entry.Metadata == nil {
		return
	}
	for key, value := range entry.Metadata {
		locale, ok := strings.CutPrefix(key, "payload_")
		if !ok {
			continue
		}
		locale = NormalizeLocale(locale)
		if locale == "" {
			continue
		}
		document, _ := value.Value.(map[string]any)
		if document == nil {
			continue
		}
		if _, exists := documents[locale]; !exists {
			documents[locale] = LocaleDocument{Data: document, Status: entry.Status}
		}
		delete(entry.Metadata, key)
	}
	SetDocuments(entry, documents)
}
