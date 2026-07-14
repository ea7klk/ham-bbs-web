package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestUtilityBusinessRules(t *testing.T) {
	if got := normalizeCallsign(" ea7klk "); got != "EA7KLK" {
		t.Fatalf("normalizeCallsign() = %q", got)
	}
	if got := normalizeLocator(" im77ah "); got != "IM77ah" {
		t.Fatalf("normalizeLocator() = %q", got)
	}
	if got := normalizeLocator("a"); got != "A" {
		t.Fatalf("normalizeLocator(short) = %q", got)
	}
	tests := map[string]string{
		"Field Operations":      "field-operations",
		"---":                   defaultBoardID,
		"":                      defaultBoardID,
		strings.Repeat("a", 45): strings.Repeat("a", 40),
	}
	for input, want := range tests {
		if got := boardID(input); got != want {
			t.Fatalf("boardID(%q) = %q, want %q", input, got, want)
		}
	}
	if got := firstNonEmpty("", "  ", " value ", "other"); got != " value " {
		t.Fatalf("firstNonEmpty() = %q", got)
	}
	if got := singleLine(" one\n two\tthree "); got != "one two three" {
		t.Fatalf("singleLine() = %q", got)
	}
	if got := asciiSafe("ok\u00e9\x00"); got != "ok??" {
		t.Fatalf("asciiSafe() = %q, want %q", got, "ok??")
	}
	users := []dbUser{{Callsign: "ZZZ"}, {Callsign: "AAA"}, {Callsign: "MMM"}}
	sortUsers(users)
	if got := []string{users[0].Callsign, users[1].Callsign, users[2].Callsign}; !reflect.DeepEqual(got, []string{"AAA", "MMM", "ZZZ"}) {
		t.Fatalf("sortUsers() = %v", got)
	}
	if _, err := time.Parse("2006-01-02 15:04 UTC", now()); err != nil {
		t.Fatalf("now() has unexpected format: %v", err)
	}
}

func TestConfigAndTranslations(t *testing.T) {
	t.Setenv("BBS_TEST_VALUE", "configured")
	if got := env("BBS_TEST_VALUE", "fallback"); got != "configured" {
		t.Fatalf("env(configured) = %q", got)
	}
	if got := env("BBS_MISSING_VALUE", "fallback"); got != "fallback" {
		t.Fatalf("env(fallback) = %q", got)
	}
	if got := parseSysops(" ea7klk, , ea1abc "); !reflect.DeepEqual(got, map[string]bool{"EA7KLK": true, "EA1ABC": true}) {
		t.Fatalf("parseSysops() = %#v", got)
	}
	choices := languageChoices()
	if len(choices) != len(languageOrder) || choices[0]["Code"] != "en" || choices[1]["Name"] != "Español" {
		t.Fatalf("languageChoices() = %#v", choices)
	}
	text := map[string]map[string]any{
		"en": {"known": "English", "wrong_type": 3},
		"es": {"known": "Español"},
	}
	for _, test := range []struct {
		lang, key, want string
	}{
		{"es", "known", "Español"},
		{"fr", "known", "English"},
		{"en", "wrong_type", "wrong_type"},
		{"en", "missing", "missing"},
	} {
		if got := translation(text, test.lang, test.key); got != test.want {
			t.Fatalf("translation(%q, %q) = %q, want %q", test.lang, test.key, got, test.want)
		}
	}
	s := &server{text: text}
	if got := s.t("es", "known"); got != "Español" {
		t.Fatalf("server.t() = %q", got)
	}
	if s.isSysop(nil) {
		t.Fatal("nil user must not be a sysop")
	}
	if s.isSysop(&dbUser{Callsign: "EA7KLK"}) {
		t.Fatal("unconfigured user must not be a sysop")
	}
	s.cfg.sysops = map[string]bool{"EA7KLK": true}
	if !s.isSysop(&dbUser{Callsign: "EA7KLK"}) {
		t.Fatal("configured sysop was not recognized")
	}

	t.Run("readJSON", func(t *testing.T) {
		var target map[string]string
		if err := readJSON(filepath.Join(t.TempDir(), "missing.json"), &target, map[string]string{"fallback": "yes"}); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(target, map[string]string{"fallback": "yes"}) {
			t.Fatalf("missing JSON fallback = %#v", target)
		}
		invalid := filepath.Join(t.TempDir(), "invalid.json")
		if err := os.WriteFile(invalid, []byte("{"), 0o644); err != nil {
			t.Fatal(err)
		}
		target = nil
		if err := readJSON(invalid, &target, map[string]string{"fallback": "yes"}); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(target, map[string]string{"fallback": "yes"}) {
			t.Fatalf("invalid JSON fallback = %#v", target)
		}
		valid := filepath.Join(t.TempDir(), "valid.json")
		data, _ := json.Marshal(map[string]string{"ok": "yes"})
		if err := os.WriteFile(valid, data, 0o644); err != nil {
			t.Fatal(err)
		}
		target = nil
		if err := readJSON(valid, &target, map[string]string{}); err != nil {
			t.Fatal(err)
		}
		if target["ok"] != "yes" {
			t.Fatalf("valid JSON = %#v", target)
		}
		if err := readJSON(t.TempDir(), &target, map[string]string{}); err == nil {
			t.Fatal("reading a directory should fail")
		}
	})
}

func TestNewServerSeedsDatabaseAndReadsEnvironment(t *testing.T) {
	dataDir := t.TempDir()
	translationFile := filepath.Join(t.TempDir(), "translations.json")
	if err := os.WriteFile(translationFile, []byte(`{"en":{"web_login":"Sign in"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BBS_DATA_DIR", dataDir)
	t.Setenv("BBS_DB_FILE", filepath.Join(dataDir, "shared.sqlite"))
	t.Setenv("BBS_TRANSLATIONS_FILE", translationFile)
	t.Setenv("BBS_WEB_ADDR", ":9999")
	t.Setenv("APRS_IS_PORT", "14581")
	t.Setenv("BBS_SYSOPS", "ea7klk")
	app, err := newServer()
	if err != nil {
		t.Fatal(err)
	}
	if app.cfg.addr != ":9999" || app.cfg.aprsPort != 14581 || !app.cfg.sysops["EA7KLK"] {
		t.Fatalf("environment was not applied: %#v", app.cfg)
	}
	var bulletins []dbBulletin
	if err := app.db.Find(&bulletins).Error; err != nil {
		t.Fatal(err)
	}
	if len(bulletins) != 2 {
		t.Fatalf("seeded bulletins = %d, want 2", len(bulletins))
	}
	var boards []dbBoard
	if err := app.db.Find(&boards).Error; err != nil {
		t.Fatal(err)
	}
	if len(boards) != 1 || boards[0].ID != defaultBoardID {
		t.Fatalf("seeded boards = %#v", boards)
	}
	if err := app.seedDefaultData(); err != nil {
		t.Fatal(err)
	}
	var count int64
	app.db.Model(&dbBulletin{}).Count(&count)
	if count != 2 {
		t.Fatalf("idempotent bulletin seed count = %d, want 2", count)
	}
}

func TestPasswordSecurity(t *testing.T) {
	hash := hashPassword("correct horse battery staple")
	if !verifyPassword("correct horse battery staple", hash) {
		t.Fatal("correct password was rejected")
	}
	if verifyPassword("wrong", hash) {
		t.Fatal("wrong password was accepted")
	}
	for _, malformed := range []string{"", "plain", "pbkdf2_sha256$not-a-number$abc$def", "other$1$abc$def", "pbkdf2_sha256$1$%%%$abc"} {
		if verifyPassword("anything", malformed) {
			t.Fatalf("malformed password hash %q was accepted", malformed)
		}
	}
	parts := strings.Split(hash, "$")
	parts[3] = base64WithoutPadding(parts[3])
	if verifyPassword("correct horse battery staple", strings.Join(parts, "$")) {
		t.Fatal("tampered password hash was accepted")
	}
	token := randomToken(24)
	if token == "" || strings.ContainsAny(token, "+/=") {
		t.Fatalf("randomToken() = %q, want raw URL-safe token", token)
	}
}

func base64WithoutPadding(value string) string {
	if value == "" {
		return "x"
	}
	return value[:len(value)-1] + "x"
}
