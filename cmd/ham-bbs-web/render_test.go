package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Page rendering tests.
//
// html/template writes partial output and only then reports an error, so a
// broken action produces a 200 with a truncated page rather than an obvious
// failure. These tests render every page and check the output is complete and
// free of the two things that make a page look broken to a user: a leaked
// translation key, and a raw template separator.

// fixtureWorld builds a database with enough content for every page to render
// its filled-in state: boards, threads, bulletins, APRS traffic and two users.
func fixtureWorld(t *testing.T) (*server, *dbUser) {
	t.Helper()
	s := newTestServer(t)
	// Render with the real strings: a key that is missing from translations.json
	// then comes back verbatim and the leak check catches it on the actual page,
	// not only in the static scan.
	text := translationsOnDisk(t)
	s.text = text
	s.tpl = parseTemplates(text)

	sysop := testUser(t, s, dbUser{
		Callsign: "EA7KLK", FullName: "Volker K", Email: "v@example.invalid",
		Maidenhead: "IM77ah", Language: "en", QTH: "Madrid", Rig: "FT-817",
		IsSysop: true, Registered: "2026-07-08 10:50 UTC",
	})
	testUser(t, s, dbUser{
		Callsign: "EA1ABC", FullName: "Ana", Email: "a@example.invalid",
		Language: "es", QTH: "Sevilla", EnableAPRS: true,
	})

	if err := s.db.Create(&dbBoard{ID: "general", Position: 0, Name: "General", Description: "General local messages", Created: now()}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.db.Create(&dbBoard{ID: "antennas", Position: 1, Name: "Antennas", Description: "Builds and SWR", Created: now()}).Error; err != nil {
		t.Fatal(err)
	}
	root, err := s.createMessage(dbMessage{BoardID: "general", From: "EA7KLK", Subject: "Net tonight", Body: "20:00 on the repeater."})
	if err != nil {
		t.Fatal(err)
	}
	for depth := 1; depth <= maxReplyDepth; depth++ {
		root, err = s.createMessage(dbMessage{ParentID: &root, From: "EA1ABC", Subject: "Re: net", Body: "Copied."})
		if err != nil {
			t.Fatalf("reply at depth %d: %v", depth, err)
		}
	}
	if err := s.db.Create(&dbBulletin{Position: 0, Title: "Net moves to 20:00", Body: "Same repeater.", Updated: now(), From: "EA7KLK"}).Error; err != nil {
		t.Fatal(err)
	}
	sent := dbAPRSSent{UserCallsign: "EA7KLK", Position: 0, At: now(), From: "EA7KLK", To: "APRS", Text: "Net tonight", Status: "sent", Acked: true, Passcode: 12345}
	if err := s.db.Create(&sent).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.db.Create(&dbAPRSSentPart{SentID: sent.ID, Number: 1, Text: "Net tonight", Status: "sent", MessageID: "1", Acked: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.db.Create(&dbAPRSReceived{UserCallsign: "EA7KLK", Position: 0, At: now(), From: "DN9RZ", To: "EA7KLK", Text: "WX back online", Raw: "DN9RZ>APRS::EA7KLK :WX back online"}).Error; err != nil {
		t.Fatal(err)
	}
	// A bulletin, so the bulletin pages render their filled state. The id is 1 in
	// a fresh database, which is what the detail-page case above asks for.
	if err := s.db.Create(&dbAPRSBulletin{
		DedupeKey: "fixture", FirstSeen: now(), LastSeen: now(), SeenCount: 4,
		From: "NWS", Slot: 1, Group: "NWS", Category: "weather",
		Text: "Tornado warning until 21:00Z",
		Raw:  "NWS>APRS::BLN1NWS  :Tornado warning until 21:00Z",
	}).Error; err != nil {
		t.Fatal(err)
	}
	return s, sysop
}

func translationsOnDisk(t *testing.T) map[string]map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "translations.json"))
	if err != nil {
		t.Fatal(err)
	}
	var text map[string]map[string]any
	if err := json.Unmarshal(data, &text); err != nil {
		t.Fatal(err)
	}
	return text
}

// leakedKeys returns translation keys that came back in the rendered HTML, which
// means a lookup missed and the user is looking at "web_something" instead of a
// sentence.
var (
	htmlTag    = regexp.MustCompile(`<[^>]*>`)
	styleBlock = regexp.MustCompile(`(?s)<style>.*?</style>`)
)

func leakedKeys(body string, text map[string]map[string]any) []string {
	known := map[string]bool{}
	for _, table := range text {
		for key := range table {
			known[key] = true
		}
	}
	seen := map[string]bool{}
	// Only visible text counts: a form field called full_name is not a leaked
	// key, so the tags are stripped before scanning.
	visible := htmlTag.ReplaceAllString(body, " ")
	// Keys are lowercase words joined by underscores; require a prefix that the
	// translation files actually use so ordinary text cannot trip the scan.
	for _, match := range regexp.MustCompile(`\b(web|menu|sysop|aprs|bulletin|board|message|account|activation|forgot|reset|email|password|user|target|full|date|thread|confirm|delete|save|send|cancel|reply|select|at|from|status|role|language|qth|rig)_[a-z0-9_]+`).FindAllString(visible, -1) {
		if known[match] {
			seen[match] = true
		}
	}
	out := make([]string, 0, len(seen))
	for key := range seen {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func TestEveryPageRenders(t *testing.T) {
	s, sysop := fixtureWorld(t)
	text := s.text

	pages := []struct {
		name    string
		path    string
		code    int
		handler func(http.ResponseWriter, *http.Request, *dbUser)
		setup   func(*http.Request) *http.Request
	}{
		// A board that does not exist is a 404, and the body is checked like any
		// other page so the error page itself has to be well formed.
		{name: "board missing", path: "/boards/nope", code: http.StatusNotFound, handler: s.boardShow, setup: pathID("id", "nope")},
		{name: "home", path: "/", handler: s.home},
		{name: "boards", path: "/boards", handler: s.boards},
		{name: "board", path: "/boards/general", handler: s.boardShow, setup: pathID("id", "general")},
		{name: "board form", path: "/boards/new", handler: s.boardNew},
		{name: "board edit", path: "/boards/general/edit", handler: s.boardEdit, setup: pathID("id", "general")},
		{name: "bulletins", path: "/bulletins", handler: s.bulletins},
		{name: "bulletin form", path: "/bulletins/new", handler: s.bulletinNew},
		{name: "bulletin edit", path: "/bulletins/1/edit", handler: s.bulletinEdit, setup: pathID("id", "1")},
		{name: "directory", path: "/directory", handler: s.directory},
		{name: "profile", path: "/profile", handler: s.profileForm},
		{name: "about", path: "/about", handler: s.about},
		{name: "resources", path: "/resources", handler: s.resources},
		{name: "admin users", path: "/admin/users", handler: s.adminUsers},
		{name: "aprs", path: "/aprs", handler: s.aprs},
		{name: "aprs received", path: "/aprs/received", handler: s.aprsReceived},
		{name: "aprs sent", path: "/aprs/sent", handler: s.aprsSent},
		{name: "aprs send", path: "/aprs/send", handler: s.aprsSendForm},
		{name: "aprs sent detail", path: "/aprs/sent/1", handler: s.aprsSentDetail, setup: pathID("id", "1")},
		{name: "aprs received detail", path: "/aprs/received/1", handler: s.aprsReceivedDetail, setup: pathID("id", "1")},
		{name: "aprs reply", path: "/aprs/received/1/reply", handler: s.aprsReceivedReplyForm, setup: pathID("id", "1")},
		{name: "message edit", path: "/messages/1/edit", handler: s.messageEditForm, setup: pathID("id", "1")},
		{name: "aprs bulletins", path: "/aprs/bulletins", handler: s.aprsBulletinIndex},
		{name: "aprs bulletin category", path: "/aprs/bulletins/weather", code: http.StatusOK, handler: s.aprsBulletinCategory, setup: pathID("category", "weather")},
		{name: "aprs bulletin category unknown", path: "/aprs/bulletins/nonsense", code: http.StatusNotFound, handler: s.aprsBulletinCategory, setup: pathID("category", "nonsense")},
		{name: "aprs bulletin detail", path: "/aprs/bulletins/weather/1", handler: s.aprsBulletinDetail, setup: func(r *http.Request) *http.Request {
			return setPathValue(setPathValue(r, "category", "weather"), "id", "1")
		}},
		{name: "aprsph", path: "/aprs/aprsph", handler: s.aprsphPage},
	}

	for _, page := range pages {
		t.Run(page.name, func(t *testing.T) {
			request := formRequest(http.MethodGet, page.path, nil)
			if page.setup != nil {
				request = page.setup(request)
			}
			response := invokeUserHandler(page.handler, request, sysop)
			want := page.code
			if want == 0 {
				want = http.StatusOK
			}
			if response.Code != want {
				t.Fatalf("status = %d, want %d", response.Code, want)
			}
			body := response.Body.String()
			if body == "" {
				t.Fatal("empty body")
			}
			// A truncated page is the tell-tale of a template execution error,
			// because html/template writes before it reports.
			if !strings.HasSuffix(strings.TrimSpace(body), "</html>") {
				t.Fatalf("page did not finish: tail = %q", tail(body, 160))
			}
			if strings.Contains(body, "{{") || strings.Contains(body, "}}") {
				t.Fatalf("raw template delimiters in output: %q", around(body, "{{"))
			}
			if keys := leakedKeys(body, text); len(keys) > 0 {
				t.Fatalf("leaked translation keys: %v", keys)
			}
			if strings.Contains(body, "ZgotmplZ") {
				t.Fatalf("template rejected a value as unsafe: %q", around(body, "ZgotmplZ"))
			}
		})
	}
}

// The public pages have no session, so they render with a nil user.
func TestPublicPagesRenderSignedOut(t *testing.T) {
	s, _ := fixtureWorld(t)
	text := s.text
	if err := s.db.Model(&dbUser{}).Where("callsign = ?", "EA7KLK").Update("activated", false).Error; err != nil {
		t.Fatal(err)
	}

	// loginForm and registerForm take no user; wrap them so every entry in the
	// table has the same shape.
	pages := map[string]func(http.ResponseWriter, *http.Request, *dbUser){
		"/login":    func(w http.ResponseWriter, r *http.Request, _ *dbUser) { s.loginForm(w, r) },
		"/register": func(w http.ResponseWriter, r *http.Request, _ *dbUser) { s.registerForm(w, r) },
		"/forgot":   s.forgotForm,
		"/activate": s.activateForm,
		"/reset":    s.resetForm,
	}
	for path, handler := range pages {
		response := invokeHandler(s.public(handler), formRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, response.Code)
		}
		body := response.Body.String()
		if !strings.HasSuffix(strings.TrimSpace(body), "</html>") {
			t.Fatalf("%s did not finish: %q", path, tail(body, 160))
		}
		if keys := leakedKeys(body, text); len(keys) > 0 {
			t.Fatalf("%s leaked translation keys: %v", path, keys)
		}
	}
}

// The redesign relies on these hooks being present: the frame, the tab bar with
// its current-section marker, and the palette. A page that lost them would still
// render, so they need an explicit check.
func TestLayoutCarriesTheDesignHooks(t *testing.T) {
	s, sysop := fixtureWorld(t)
	body := invokeUserHandler(s.home, formRequest(http.MethodGet, "/", nil), sysop).Body.String()

	for _, want := range []string{
		`class="frame"`,       // the ruled page frame
		`class="head"`,        // title block
		`class="tabs"`,        // section tabs
		`aria-current="page"`, // current section, for screen readers too
		`--cyan:#4fd8ff`,      // Spectrum palette
		`--violet:#8b7bff`,
		`--pink:#ff6ec7`,
		`--lime:#b6f36a`,
		`color-scheme: dark`,
		`font-family: var(--mono)`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("home page is missing %q", want)
		}
	}
	// The home tab is lit on the home page, and only there. The stylesheet also
	// mentions the attribute selector, so count only the markup.
	markup := styleBlock.ReplaceAllString(body, "")
	if got := strings.Count(markup, `aria-current="page"`); got != 1 {
		t.Fatalf("expected exactly one current tab, got %d", got)
	}
}

// The tab bar must show where the caller is: a board page lights Boards, an APRS
// sub-page lights APRS, and home is lit only on home.
func TestSectionTabFollowsThePath(t *testing.T) {
	s, sysop := fixtureWorld(t)
	handler := s.routes()

	response := httptest.NewRecorder()
	s.setSession(response, sysop.Callsign)
	cookie := response.Result().Cookies()[0]

	for _, test := range []struct {
		path string
		tab  string
	}{
		{"/", "/"},
		{"/boards", "/boards"},
		{"/boards/general", "/boards"},
		{"/bulletins", "/bulletins"},
		{"/directory", "/directory"},
		{"/aprs", "/aprs"},
		{"/aprs/sent", "/aprs"},
		{"/admin/users", "/admin/users"},
		{"/profile", "/profile"},
	} {
		request := httptest.NewRequest(http.MethodGet, test.path, nil)
		request.AddCookie(cookie)
		page := httptest.NewRecorder()
		handler.ServeHTTP(page, request)
		if page.Code != http.StatusOK {
			t.Fatalf("%s status = %d", test.path, page.Code)
		}
		markup := styleBlock.ReplaceAllString(page.Body.String(), "")
		want := `href="` + test.tab + `" aria-current="page"`
		if !strings.Contains(markup, want) {
			t.Fatalf("%s did not light the %s tab", test.path, test.tab)
		}
		if got := strings.Count(markup, `aria-current="page"`); got != 1 {
			t.Fatalf("%s lit %d tabs", test.path, got)
		}
	}
}

func pathID(name, value string) func(*http.Request) *http.Request {
	return func(r *http.Request) *http.Request { return setPathValue(r, name, value) }
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func around(s, needle string) string {
	i := strings.Index(s, needle)
	if i < 0 {
		return ""
	}
	start := i - 60
	if start < 0 {
		start = 0
	}
	end := i + 60
	if end > len(s) {
		end = len(s)
	}
	return s[start:end]
}

// The login page tells a signed-out caller that the board is also reachable over
// SSH, with the command ready to copy. It is a fixed command for this
// deployment, so the test pins the exact string rather than a shape.
func TestLoginPageShowsTheSSHCommand(t *testing.T) {
	s, sysop := fixtureWorld(t)
	response := invokeHandler(s.loginForm, formRequest(http.MethodGet, "/login", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("login status = %d", response.Code)
	}
	body := response.Body.String()

	if !strings.Contains(body, "ssh -p 2222 bbs@bbs.ea7klk.es") {
		t.Fatal("the login page does not show the SSH command")
	}
	if !strings.Contains(body, `class="sshhint"`) {
		t.Fatal("the SSH hint is not wearing its own class")
	}
	// The note is a sign-in hint, not chrome: it must not appear once signed in.
	home := invokeUserHandler(s.home, formRequest(http.MethodGet, "/", nil), sysop)
	if strings.Contains(home.Body.String(), "ssh -p 2222") {
		t.Fatal("the SSH hint leaked into the signed-in pages")
	}
}

// The note is translated; every language must have it.
func TestSSHNoteIsTranslated(t *testing.T) {
	text := translationsOnDisk(t)
	for _, lang := range languageOrder {
		value, ok := text[lang]["web_ssh_note"].(string)
		if !ok || strings.TrimSpace(value) == "" {
			t.Fatalf("web_ssh_note is missing for %q", lang)
		}
	}
}
