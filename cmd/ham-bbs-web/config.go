package main

import (
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
	sentHistoryLimit     = 200
	receivedHistoryLimit = 500
)

var (
	callsignRE      = regexp.MustCompile(`^[A-Z0-9][A-Z0-9/-]{2,15}$`)
	aprsCallsignRE  = regexp.MustCompile(`^[A-Z0-9]{1,10}(-[0-9]{1,2})?$`)
	emailRE         = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
	maidenheadRE    = regexp.MustCompile(`^[A-Ra-r]{2}([0-9]{2}([A-Xa-x]{2}([0-9]{2}([A-Xa-x]{2})?)?)?)?$`)
	boardIDRE       = regexp.MustCompile(`[^a-z0-9]+`)
	languages       = map[string]string{"en": "English", "es": "Espanol", "fr": "Francais", "de": "Deutsch"}
	languageOrder   = []string{"en", "es", "fr", "de"}
	errUnauthorized = errors.New("unauthorized")
)

type config struct {
	addr       string
	dataDir    string
	dbFile     string
	aprsLog    string
	bbsLog     string
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
	sessions map[string]string
	mu       sync.RWMutex
}

type viewData struct {
	AppName   string
	Location  string
	Topic     string
	SysopName string
	User      *dbUser
	IsSysop   bool
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
	app := &server{cfg: cfg, db: db, tpl: parseTemplates(), sessions: map[string]string{}}
	if err := app.seedDefaultData(); err != nil {
		return nil, err
	}
	return app, nil
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
