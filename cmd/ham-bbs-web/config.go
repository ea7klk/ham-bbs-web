package main

import (
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

const (
	defaultBoardID       = "general"
	passwordIterations   = 200000
	aprsMessageLimit     = 67
	aprsBeaconText       = "HamNet BBS"
	sentHistoryLimit     = 200
	receivedHistoryLimit = 500
)

var (
	callsignRE      = regexp.MustCompile(`^[A-Z0-9][A-Z0-9/-]{2,15}$`)
	aprsCallsignRE  = regexp.MustCompile(`^[A-Z0-9]{1,10}(-[0-9]{1,2})?$`)
	emailRE         = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	maidenheadRE    = regexp.MustCompile(`^[A-Ra-r]{2}([0-9]{2}([A-Xa-x]{2}([0-9]{2}([A-Xa-x]{2})?)?)?)?$`)
	boardIDRE       = regexp.MustCompile(`[^a-z0-9]+`)
	languages       = map[string]string{"en": "English", "es": "Español", "fr": "Français", "de": "Deutsch"}
	languageOrder   = []string{"en", "es", "fr", "de"}
	errUnauthorized = errors.New("unauthorized")
)

type config struct {
	addr       string
	dataDir    string
	dbFile     string
	aprsLog    string
	bbsLog     string
	transFile  string
	name       string
	sysopName  string
	sysops     map[string]bool
	location   string
	topic      string
	aprsServer string
	aprsPort   int
}

type server struct {
	cfg      config
	db       *gorm.DB
	tpl      *template.Template
	text     map[string]map[string]any
	sessions map[string]string
	mu       sync.RWMutex
}

type viewData struct {
	AppName   string
	Location  string
	Topic     string
	SysopName string
	Path      string
	User      *dbUser
	IsSysop   bool
	Lang      string
	Flash     string
	Error     string
	Data      any
}

func (s *server) isSysop(user *dbUser) bool {
	return user != nil && (user.IsSysop || s.cfg.sysops[user.Callsign])
}

func newServer() (*server, error) {
	dataDir := env("BBS_DATA_DIR", "/var/lib/bbs")
	port, _ := strconv.Atoi(env("APRS_IS_PORT", "14580"))
	cfg := config{
		addr:       env("BBS_WEB_ADDR", ":8080"),
		dataDir:    dataDir,
		dbFile:     env("BBS_DB_FILE", filepath.Join(dataDir, "bbs.sqlite")),
		aprsLog:    filepath.Join(dataDir, "aprs", "aprs.log"),
		bbsLog:     filepath.Join(dataDir, "bbs.log"),
		transFile:  env("BBS_TRANSLATIONS_FILE", defaultTranslationsFile()),
		name:       env("BBS_NAME", "HAMNET RADIO BBS"),
		sysopName:  env("BBS_SYSOP", "Sysop"),
		sysops:     parseSysops(os.Getenv("BBS_SYSOPS")),
		location:   env("BBS_LOCATION", "HamNet"),
		topic:      env("BBS_WELCOME_TOPIC", "Amateur radio notes, local nets, and packet-era experiments"),
		aprsServer: env("APRS_IS_SERVER", "rotate.aprs2.net"),
		aprsPort:   port,
	}
	if err := os.MkdirAll(filepath.Join(dataDir, "aprs"), 0o755); err != nil {
		return nil, err
	}
	db, err := openDatabase(cfg.dbFile)
	if err != nil {
		return nil, err
	}
	text := map[string]map[string]any{}
	if err := readJSON(cfg.transFile, &text, map[string]map[string]any{}); err != nil {
		return nil, err
	}
	app := &server{cfg: cfg, db: db, text: text, tpl: parseTemplates(text), sessions: map[string]string{}}
	if err := app.seedDefaultData(); err != nil {
		return nil, err
	}
	return app, nil
}

func defaultTranslationsFile() string {
	if _, err := os.Stat("translations.json"); err == nil {
		return "translations.json"
	}
	return "/usr/local/share/ham-bbs-web/translations.json"
}

func readJSON[T any](path string, target *T, fallback T) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		*target = fallback
		return nil
	}
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, target); err != nil {
		*target = fallback
		return nil
	}
	return nil
}

func translation(text map[string]map[string]any, lang, key string) string {
	if byLang, ok := text[lang]; ok {
		if value, ok := byLang[key].(string); ok {
			return value
		}
	}
	if byLang, ok := text["en"]; ok {
		if value, ok := byLang[key].(string); ok {
			return value
		}
	}
	return key
}

func (s *server) t(lang, key string) string {
	return translation(s.text, lang, key)
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func parseSysops(raw string) map[string]bool {
	sysops := map[string]bool{}
	for _, item := range strings.Split(raw, ",") {
		item = normalizeCallsign(item)
		if item != "" {
			sysops[item] = true
		}
	}
	return sysops
}

func languageChoices() []map[string]string {
	out := []map[string]string{}
	for _, code := range languageOrder {
		out = append(out, map[string]string{"Code": code, "Name": languages[code]})
	}
	return out
}
