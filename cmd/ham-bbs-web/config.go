package main

import (
	"encoding/json"
	"errors"
	"gorm.io/gorm"
	"html/template"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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

// maxReplyDepth bounds how deep replies may nest below a top-level message, so
// a thread is at most maxReplyDepth+1 rows deep. The classic SSH BBS enforces
// the same cap against the shared database, so the web app must not be the
// bypass that stores deeper threads.
const maxReplyDepth = 5

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
	throttle *authThrottle
	// mail sends the activation codes, reset links and email-change links that
	// the account flows need. A deployment without SMTP configured runs with
	// activation skipped, exactly as the SSH BBS does.
	mail *mailer
	mu   sync.RWMutex
	// background tracks fire-and-forget work (such as the login beacon) so a
	// test can wait for it before removing its temporary directory.
	background sync.WaitGroup
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

// storedActiveSysops counts enabled accounts that hold the sysop role in the
// database, rather than in a snapshot read earlier.
func (s *server) storedActiveSysops() (int64, error) {
	var count int64
	err := s.db.Model(&dbUser{}).Where("is_sysop = ? AND disabled = ?", true, false).Count(&count).Error
	return count, err
}

// isBootstrapSysop reports whether callsign may be granted sysop because it is
// listed in BBS_SYSOPS and the deployment has no active sysop yet. It fails
// closed: if the count cannot be read, no role is granted.
func (s *server) isBootstrapSysop(callsign string) bool {
	if !s.cfg.sysops[normalizeCallsign(callsign)] {
		return false
	}
	count, err := s.storedActiveSysops()
	if err != nil {
		return false
	}
	return count == 0
}

// applyConfiguredSysops promotes existing accounts listed in BBS_SYSOPS only
// while the database has no active sysop, and returns the callsigns it changed.
// Once a sysop exists the configured list has no authority, so a configured
// callsign can be demoted like any other.
func (s *server) applyConfiguredSysops() []string {
	count, err := s.storedActiveSysops()
	if err != nil || count > 0 {
		return nil
	}
	changed := []string{}
	for callsign := range s.cfg.sysops {
		var users []dbUser
		if err := s.db.Where("callsign = ?", callsign).Limit(1).Find(&users).Error; err != nil {
			continue
		}
		if len(users) == 0 || users[0].IsSysop {
			continue
		}
		if err := s.db.Model(&dbUser{}).Where("callsign = ?", callsign).Update("is_sysop", true).Error; err != nil {
			continue
		}
		changed = append(changed, callsign)
	}
	sort.Strings(changed)
	return changed
}

// isSysop answers from the stored account alone. BBS_SYSOPS is deliberately not
// consulted here: it is a bootstrap mechanism, used once to grant the first
// sysop, and has no authority afterwards. This mirrors the classic SSH BBS
// (simple-ham-bbs, bbs/cmd/bbs/storage.go).
func (s *server) isSysop(user *dbUser) bool {
	return user != nil && user.IsSysop
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
	app := &server{cfg: cfg, db: db, text: text, tpl: parseTemplates(text), sessions: map[string]string{}, throttle: newAuthThrottle(), mail: newMailerFromEnv()}
	if problem := app.mail.Problem(); problem != "" {
		// Say it once at startup rather than failing every send later.
		log.Printf("outbound mail disabled: %s", problem)
	}
	if err := app.seedDefaultData(); err != nil {
		return nil, err
	}
	for _, callsign := range app.applyConfiguredSysops() {
		app.logBBSAction("system", "web_sysop_bootstrap", "callsign=%q", callsign)
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
