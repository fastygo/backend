package media

import (
	"strings"
	"testing"

	"github.com/fastygo/backend/internal/domain/content"
)

func TestFromEntryAndValidation(t *testing.T) {
	t.Parallel()
	checksum := strings.Repeat("ab", 32)
	valid := content.Entry{
		ID: "media_1", Kind: "media", Status: content.StatusPublished, Visibility: content.VisibilityPublic,
		Title: content.LocalizedText{"en": "Cover"},
		Metadata: map[string]content.MetadataValue{
			MetadataFilename:   {Value: "cover.png"},
			MetadataMIMEType:   {Value: "image/png"},
			MetadataSize:       {Value: int64(128)},
			MetadataChecksum:   {Value: checksum},
			MetadataStorageKey: {Value: "media/cover.png"},
		},
		Version: 1,
	}
	cases := map[string]struct {
		mutate    func(*content.Entry)
		wantError bool
	}{
		"valid int64 size": {},
		"float size": {
			mutate: func(entry *content.Entry) {
				entry.Metadata[MetadataSize] = content.MetadataValue{Value: float64(64)}
			},
		},
		"not media kind": {
			mutate:    func(entry *content.Entry) { entry.Kind = content.KindPost },
			wantError: true,
		},
		"invalid size": {
			mutate: func(entry *content.Entry) {
				entry.Metadata[MetadataSize] = content.MetadataValue{Value: "big"}
			},
			wantError: true,
		},
		"short checksum": {
			mutate: func(entry *content.Entry) {
				entry.Metadata[MetadataChecksum] = content.MetadataValue{Value: "deadbeef"}
			},
			wantError: true,
		},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			entry := valid
			entry.Metadata = cloneMetadata(valid.Metadata)
			if test.mutate != nil {
				test.mutate(&entry)
			}
			asset, err := FromEntry(entry)
			if test.wantError {
				if err == nil {
					t.Fatalf("expected media validation error")
				}
				return
			}
			if err != nil {
				t.Fatalf("valid media rejected: %v", err)
			}
			if asset.Filename != "cover.png" || asset.StorageKey == "" {
				t.Fatalf("unexpected asset: %#v", asset)
			}
		})
	}
}

func cloneMetadata(source map[string]content.MetadataValue) map[string]content.MetadataValue {
	cloned := make(map[string]content.MetadataValue, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}
