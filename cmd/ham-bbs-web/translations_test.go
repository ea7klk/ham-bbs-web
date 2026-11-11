package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// translationsFile is the file shipped in the image and loaded at runtime.
func translationsFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "translations.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("translations.json not found at %s: %v", path, err)
	}
	return path
}

func loadTranslations(t *testing.T) map[string]map[string]any {
	t.Helper()
	data, err := os.ReadFile(translationsFile(t))
	if err != nil {
		t.Fatal(err)
	}
	var text map[string]map[string]any
	if err := json.Unmarshal(data, &text); err != nil {
		t.Fatalf("translations.json is not valid JSON: %v", err)
	}
	return text
}

// Every language must define the same keys, so no screen falls back to a raw
// key for one locale.
func TestAllTranslationsHaveTheSameKeys(t *testing.T) {
	text := loadTranslations(t)
	if len(text) == 0 {
		t.Fatal("translations.json is empty")
	}
	for _, code := range languageOrder {
		if _, ok := text[code]; !ok {
			t.Fatalf("language %q is missing from translations.json", code)
		}
	}
	reference := text["en"]
	for _, code := range languageOrder {
		for key := range reference {
			if _, ok := text[code][key]; !ok {
				t.Fatalf("key %q is missing from language %q", key, code)
			}
		}
		for key := range text[code] {
			if _, ok := reference[key]; !ok {
				t.Fatalf("language %q has key %q that English does not", code, key)
			}
		}
	}
}

// Every key a template or handler asks for must exist, otherwise the literal key
// is rendered to the user.
func TestEveryReferencedTranslationKeyExists(t *testing.T) {
	text := loadTranslations(t)
	used := map[string]bool{}

	// Template call sites: {{tr .Lang "key"}} and the two-argument helpers that
	// build a key from a value (aprs_sent_status_x) are checked separately.
	templateKeys := regexp.MustCompile(`(?:\btr|\bs\.t)\s+[^"']*"([a-z0-9_]+)"`)
	templateSources := []string{templates}
	for _, source := range templateSources {
		for _, match := range templateKeys.FindAllStringSubmatch(source, -1) {
			used[match[1]] = true
		}
	}
	// Handler call sites: s.t(lang, "key").
	handlerKeys := regexp.MustCompile(`s\.t\([^,]+,\s*"([a-z0-9_]+)"\)`)
	for _, file := range mustGlob(t, "*.go") {
		if regexp.MustCompile(`_test\.go$`).MatchString(file) {
			continue
		}
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, match := range handlerKeys.FindAllStringSubmatch(string(source), -1) {
			used[match[1]] = true
		}
	}

	if len(used) < 20 {
		t.Fatalf("only %d translation keys found; the scanner is probably wrong", len(used))
	}
	for key := range used {
		for _, code := range languageOrder {
			if _, ok := text[code][key]; !ok {
				t.Fatalf("key %q is used but missing from language %q", key, code)
			}
		}
	}

	// The keys added by the compatibility work must be present, because they are
	// shown exactly when a user has hit a limit.
	for _, key := range []string{"thread_too_deep", "web_login_locked"} {
		if !used[key] {
			t.Fatalf("key %q is not referenced anywhere; the parity check lost track of it", key)
		}
	}
}

func mustGlob(t *testing.T, pattern string) []string {
	t.Helper()
	files, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatalf("no files matched %q", pattern)
	}
	return files
}

// The bulletin category labels and descriptions are built from an id at runtime
// ("aprs_bulletin_category_" + id), so the scanner above cannot see them. Every
// category the reader can be shown must exist in every language, or the page
// shows a raw key exactly where it matters most — a weather warning.
func TestBulletinCategoriesAreTranslated(t *testing.T) {
	text := loadTranslations(t)
	if len(aprsBulletinCategories) != 13 {
		t.Fatalf("expected 13 bulletin categories, found %d", len(aprsBulletinCategories))
	}
	for _, code := range languageOrder {
		for _, category := range aprsBulletinCategories {
			for _, prefix := range []string{"aprs_bulletin_category_", "aprs_bulletin_desc_"} {
				key := prefix + category
				value, ok := text[code][key].(string)
				if !ok || strings.TrimSpace(value) == "" {
					t.Fatalf("%s is missing from language %q", key, code)
				}
			}
		}
	}
}

// The APRSPH help lines are an array, not a string, so the string scanner skips
// them. The page renders every element, so every language needs the array.
func TestAPRSPHHelpLinesAreTranslated(t *testing.T) {
	text := loadTranslations(t)
	for _, code := range languageOrder {
		raw, ok := text[code]["aprsph_help_lines"].([]any)
		if !ok || len(raw) == 0 {
			t.Fatalf("aprsph_help_lines is missing or empty for %q", code)
		}
		for i, line := range raw {
			value, ok := line.(string)
			if !ok || strings.TrimSpace(value) == "" {
				t.Fatalf("aprsph_help_lines[%d] is not a string for %q", i, code)
			}
		}
	}
}
