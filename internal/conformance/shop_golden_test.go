package conformance_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	application "github.com/fastygo/backend/internal/application/content"
	applicationtaxonomy "github.com/fastygo/backend/internal/application/taxonomy"
	"github.com/fastygo/backend/internal/delivery/rest"
	"github.com/fastygo/backend/internal/domain/schema"
	bboltstorage "github.com/fastygo/backend/internal/storage/bbolt"
	"github.com/fastygo/codex/canonical"
	domaincontent "github.com/fastygo/codex/content"
	"github.com/fastygo/codex/taxonomy"
)

// TestCodexV1ShopGoldenHTTP boots /codex/v1 with the vendored latte-shop Codex
// records (wp-fasty shop demo goldens) and compares each golden file's endpoint
// body byte for byte. request_id is ignored.
func TestCodexV1ShopGoldenHTTP(t *testing.T) {
	root := filepath.Join("testdata", "wp-fasty-latte-shop")
	goldenDir := filepath.Join(root, "golden")
	files, err := os.ReadDir(goldenDir)
	if err != nil {
		t.Fatalf("golden dir: %v", err)
	}
	manifestJSON, err := os.ReadFile(filepath.Join(root, "codex.manifest.json"))
	if err != nil {
		t.Fatalf("shop Codex manifest: %v", err)
	}
	deliveryJSON, err := os.ReadFile(filepath.Join(root, "codex.delivery.json"))
	if err != nil {
		t.Fatalf("shop Codex delivery: %v", err)
	}
	var delivery map[string]any
	if err := json.Unmarshal(deliveryJSON, &delivery); err != nil {
		t.Fatalf("delivery json: %v", err)
	}
	if _, ok := delivery["wordpress"]; ok {
		t.Fatal("vendored delivery JSON still has key wordpress; it must be wp")
	}
	if _, ok := rest.CodexDeliverySection(delivery); !ok {
		t.Fatal("vendored delivery JSON missing wp section")
	}

	adapter, err := bboltstorage.Open(filepath.Join(t.TempDir(), "shop-golden.db"), 0o600, nil)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { _ = adapter.Close() })
	service, err := application.NewService(adapter, nil, nil)
	if err != nil {
		t.Fatalf("content service: %v", err)
	}
	taxonomies, err := applicationtaxonomy.NewService(adapter, nil)
	if err != nil {
		t.Fatalf("taxonomy service: %v", err)
	}
	manifest := schema.WithCoreResources(schema.Manifest{
		Name: "latte-shop", Version: "1.0.0",
		Resources: []schema.Resource{{
			ID: "product", Collection: "products", Public: true, RESTVisible: true,
			Taxonomies: []string{"product_cat"},
		}},
	})
	if err := service.SetManifest(manifest); err != nil {
		t.Fatalf("set manifest: %v", err)
	}
	handler, err := rest.NewCodexHandler(service, taxonomies, nil, manifest)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	handler.SetServedCodexManifestJSON(manifestJSON)
	handler.SetAvailableLocales([]string{"en-US"})
	mux := http.NewServeMux()
	handler.Routes(mux)

	ctx := context.Background()
	if err := loadShopTaxonomies(ctx, adapter, goldenDir); err != nil {
		t.Fatalf("load taxonomies: %v", err)
	}
	if err := loadShopEntries(ctx, adapter, goldenDir); err != nil {
		t.Fatalf("load entries: %v", err)
	}

	locale, settingsID, err := shopSettingsLocale(goldenDir)
	if err != nil {
		t.Fatalf("settings locale: %v", err)
	}
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	type result struct {
		file   string
		path   string
		match  bool
		detail string
	}
	var results []result
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}
		name := file.Name()
		path, ok := shopGoldenPath(name, locale, settingsID)
		if !ok {
			t.Fatalf("unmapped golden %s", name)
		}
		golden, err := os.ReadFile(filepath.Join(goldenDir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if bytes.Contains(golden, []byte(`"counts"`)) {
			t.Fatalf("%s still has counts; refresh goldens from wp-fasty 70dae86+", name)
		}
		resp, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: HTTP %d %s", path, resp.StatusCode, body)
		}
		got := stripRequestID(body)
		match := bytes.Equal(got, golden)
		detail := ""
		if !match {
			detail = fmt.Sprintf("got %s\nwant %s", got, golden)
		}
		results = append(results, result{file: name, path: path, match: match, detail: detail})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].file < results[j].file })
	matched := 0
	for _, item := range results {
		if item.match {
			matched++
		}
		status := "MATCH"
		if !item.match {
			status = "DIFF"
		}
		t.Logf("%s %s %s", status, item.file, item.path)
		if !item.match {
			t.Errorf("%s %s\n%s", item.file, item.path, item.detail)
		}
	}
	t.Logf("shop golden HTTP: %d of %d", matched, len(results))
	if matched != len(results) {
		t.Fatalf("shop golden HTTP: %d of %d matched", matched, len(results))
	}
}

func loadShopTaxonomies(ctx context.Context, adapter *bboltstorage.Adapter, goldenDir string) error {
	raw, err := os.ReadFile(filepath.Join(goldenDir, "taxonomies.json"))
	if err != nil {
		return err
	}
	var envelope struct {
		Data []taxonomy.Definition `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return err
	}
	if err := adapter.WithinTaxonomyTransaction(ctx, func(transaction applicationtaxonomy.Transaction) error {
		for _, definition := range envelope.Data {
			if err := transaction.Taxonomies().SaveDefinition(ctx, definition, 0); err != nil {
				return fmt.Errorf("definition %s: %w", definition.ID, err)
			}
		}
		return nil
	}); err != nil {
		return err
	}
	termFiles, err := filepath.Glob(filepath.Join(goldenDir, "terms.*.json"))
	if err != nil {
		return err
	}
	sort.Strings(termFiles)
	return adapter.WithinTaxonomyTransaction(ctx, func(transaction applicationtaxonomy.Transaction) error {
		for _, file := range termFiles {
			body, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			var envelope struct {
				Data []taxonomy.Term `json:"data"`
			}
			if err := json.Unmarshal(body, &envelope); err != nil {
				return fmt.Errorf("%s: %w", filepath.Base(file), err)
			}
			for _, term := range envelope.Data {
				if err := transaction.Taxonomies().SaveTerm(ctx, term, 0); err != nil {
					return fmt.Errorf("term %s: %w", term.ID, err)
				}
			}
		}
		return nil
	})
}

func loadShopEntries(ctx context.Context, adapter *bboltstorage.Adapter, goldenDir string) error {
	entryFiles, err := filepath.Glob(filepath.Join(goldenDir, "entry.*.json"))
	if err != nil {
		return err
	}
	sort.Strings(entryFiles)
	settingsRaw, err := os.ReadFile(filepath.Join(goldenDir, "settings.json"))
	if err != nil {
		return err
	}
	settingsEntry, err := settingsEntryFromGolden(settingsRaw)
	if err != nil {
		return err
	}
	return adapter.WithinContentTransaction(ctx, func(transaction application.Transaction) error {
		for _, file := range entryFiles {
			body, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			var entry domaincontent.Entry
			if err := json.Unmarshal(body, &entry); err != nil {
				return fmt.Errorf("%s: %w", filepath.Base(file), err)
			}
			if err := transaction.Content().Create(ctx, entry); err != nil {
				return fmt.Errorf("entry %s: %w", entry.ID, err)
			}
		}
		return transaction.Content().Create(ctx, settingsEntry)
	})
}

func settingsEntryFromGolden(raw []byte) (domaincontent.Entry, error) {
	var settings domaincontent.Settings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return domaincontent.Entry{}, err
	}
	published := settings.CreatedAt.UTC()
	metadata := map[string]domaincontent.MetadataValue{}
	putString := func(key, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		metadata[key] = domaincontent.MetadataValue{Value: value}
	}
	putString("language", settings.Language)
	putString("timezone", settings.Timezone)
	putString("logo_media_id", settings.LogoMediaID)
	putString("icon_media_id", settings.IconMediaID)
	putString("front_page_id", settings.FrontPageID)
	putString("posts_page_id", settings.PostsPageID)
	putString("currency", settings.Currency)
	if settings.PostsPerPage != nil {
		metadata["posts_per_page"] = domaincontent.MetadataValue{Value: *settings.PostsPerPage}
	}
	if settings.CommentsOpen != nil {
		metadata["comments_open"] = domaincontent.MetadataValue{Value: *settings.CommentsOpen}
	}
	return domaincontent.Entry{
		ID: settings.ID, Kind: domaincontent.KindSetting, Locale: settings.Locale,
		Status: domaincontent.StatusPublished, Visibility: domaincontent.VisibilityPublic,
		Slug: string(settings.ID), Title: settings.Title, Excerpt: settings.Description,
		Metadata: metadata, Origin: settings.Origin, Version: settings.Version,
		CreatedAt: settings.CreatedAt, UpdatedAt: settings.UpdatedAt, PublishedAt: &published,
	}, nil
}

func shopSettingsLocale(goldenDir string) (locale, id string, err error) {
	raw, err := os.ReadFile(filepath.Join(goldenDir, "settings.json"))
	if err != nil {
		return "", "", err
	}
	var settings struct {
		ID       string `json:"id"`
		Language string `json:"language"`
	}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return "", "", err
	}
	if settings.Language == "" || settings.ID == "" {
		return "", "", fmt.Errorf("settings.json missing id/language")
	}
	return settings.Language, settings.ID, nil
}

func shopGoldenPath(name, locale, settingsID string) (string, bool) {
	loc := url.QueryEscape(locale)
	switch {
	case name == "discovery.json":
		return "/codex/v1/", true
	case name == "taxonomies.json":
		return "/codex/v1/taxonomies?locale=" + loc, true
	case name == "settings.list.json":
		return "/codex/v1/settings?locale=" + loc, true
	case name == "settings.json":
		return "/codex/v1/settings/" + settingsID + "?locale=" + loc, true
	case strings.HasPrefix(name, "entries."):
		rest := strings.TrimSuffix(strings.TrimPrefix(name, "entries."), ".json")
		kind, page, ok := strings.Cut(rest, ".")
		if !ok {
			return "", false
		}
		return "/codex/v1/entries/" + kind + "?locale=" + loc + "&per_page=100&page=" + page, true
	case strings.HasPrefix(name, "entry."):
		id := strings.TrimSuffix(strings.TrimPrefix(name, "entry."), ".json")
		cut := strings.LastIndex(id, "-")
		if cut <= 0 {
			return "", false
		}
		return "/codex/v1/entries/" + id[:cut] + "/" + id + "?locale=" + loc, true
	case strings.HasPrefix(name, "terms."):
		id := strings.TrimSuffix(strings.TrimPrefix(name, "terms."), ".json")
		return "/codex/v1/terms/" + id + "?locale=" + loc + "&per_page=100", true
	default:
		return "", false
	}
}

func stripRequestID(body []byte) []byte {
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		return body
	}
	errorBody, _ := document["error"].(map[string]any)
	if errorBody == nil {
		return body
	}
	if _, ok := errorBody["request_id"]; !ok {
		return body
	}
	delete(errorBody, "request_id")
	document["error"] = errorBody
	raw, err := json.Marshal(document)
	if err != nil {
		return body
	}
	text, err := canonical.CanonicalJSON(string(raw))
	if err != nil {
		return body
	}
	return []byte(text)
}
