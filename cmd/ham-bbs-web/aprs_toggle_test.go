package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// The APRS switch.
//
// It used to be a `<span class="chip">` in the page header: styled like a
// control, doing nothing. The real toggle lived on the *send* page, which is the
// last place anyone would look for a setting. These tests pin the thing that was
// actually wrong — that the control on the APRS page is a control — and that the
// flag it sets has an effect.

func aprsPage(t *testing.T, s *server, user *dbUser) string {
	t.Helper()
	response := invokeUserHandler(s.aprs, formRequest(http.MethodGet, "/aprs", nil), user)
	if response.Code != http.StatusOK {
		t.Fatalf("APRS page status = %d", response.Code)
	}
	return response.Body.String()
}

// The control must be a form posting to /aprs/toggle with a button, not a label.
func TestAPRSPageCarriesARealToggle(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Op", Email: "o@example.invalid"})

	body := aprsPage(t, s, user)

	if !strings.Contains(body, `action="/aprs/toggle"`) {
		t.Fatal("the APRS page has no form posting to /aprs/toggle")
	}
	if !strings.Contains(body, `method="post"`) {
		t.Fatal("the APRS toggle is not a POST form")
	}
	// A button inside that form, not a decorative span.
	form := regexp.MustCompile(`(?s)<form[^>]*action="/aprs/toggle".*?</form>`).FindString(body)
	if form == "" {
		t.Fatal("could not isolate the toggle form")
	}
	if !strings.Contains(form, "<button") {
		t.Fatalf("the toggle form has no submit button: %q", form)
	}
	// And the old decoration is gone.
	if strings.Contains(body, `class="chip"`) {
		t.Fatal("the page still renders the decorative chip that looked like a button")
	}
}

// One click must be enough: the form carries the value to write, so there is no
// separate Save step to forget.
func TestAPRSToggleButtonNamesTheActionAndCarriesTheValue(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Op", Email: "o@example.invalid"})

	off := aprsPage(t, s, user)
	if !strings.Contains(off, `name="enable_aprs" value="true"`) {
		t.Fatalf("while APRS is off the form must submit true: %q", off)
	}
	if !strings.Contains(off, "web_aprs_enable_action") {
		t.Fatalf("the button does not offer to enable: %q", off)
	}

	if err := s.db.Model(&dbUser{}).Where("callsign = ?", user.Callsign).Update("enable_aprs", true).Error; err != nil {
		t.Fatal(err)
	}
	on := invokeUserHandler(s.aprs, formRequest(http.MethodGet, "/aprs", nil), &dbUser{Callsign: "EA1ABC", FullName: "Op", Email: "o@example.invalid", Language: "en", EnableAPRS: true}).Body.String()
	if !strings.Contains(on, `name="enable_aprs" value="false"`) {
		t.Fatalf("while APRS is on the form must submit false: %q", on)
	}
	if !strings.Contains(on, "web_aprs_disable_action") {
		t.Fatalf("the button does not offer to disable: %q", on)
	}
}

// Toggling from the APRS page must come back to the APRS page, not to the send
// form.
func TestAPRSToggleReturnsToThePageItCameFrom(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Op", Email: "o@example.invalid"})

	response := invokeUserHandler(s.aprsToggle, formRequest(http.MethodPost, "/aprs/toggle", url.Values{
		"enable_aprs": {"true"},
		"return_to":   {"/aprs"},
	}), user)
	assertRedirect(t, response, "/aprs")

	var saved dbUser
	if err := s.db.First(&saved, "callsign = ?", "EA1ABC").Error; err != nil {
		t.Fatal(err)
	}
	if !saved.EnableAPRS {
		t.Fatal("the toggle did not enable APRS")
	}

	// And off again, from the APRS page.
	response = invokeUserHandler(s.aprsToggle, formRequest(http.MethodPost, "/aprs/toggle", url.Values{
		"enable_aprs": {"false"},
		"return_to":   {"/aprs"},
	}), &saved)
	assertRedirect(t, response, "/aprs")
	if err := s.db.First(&saved, "callsign = ?", "EA1ABC").Error; err != nil {
		t.Fatal(err)
	}
	if saved.EnableAPRS {
		t.Fatal("the toggle did not disable APRS")
	}
}

// The return field is attacker-controllable, so it must not become a redirect to
// anywhere.
func TestAPRSToggleReturnTargetIsLimited(t *testing.T) {
	cases := map[string]string{
		"/aprs":                "/aprs",
		"/aprs/sent":           "/aprs/sent",
		"/aprs/received":       "/aprs/received",
		"":                     "/aprs",
		"/admin/users":         "/aprs",
		"//evil.example":       "/aprs",
		"https://evil.example": "/aprs",
		"/aprsx":               "/aprs",
		"/aprs/../admin/users": "/aprs/../admin/users",
		"  /aprs  ":            "/aprs",
	}
	for input, want := range cases {
		// The traversal case is harmless: it is a path on this server, and no
		// handler there accepts it.
		if got := aprsReturnTarget(input); got != want {
			t.Fatalf("aprsReturnTarget(%q) = %q, want %q", input, got, want)
		}
	}
}

// With APRS off, nothing may transmit: not the send form, not a POST straight at
// the route, not a reply. This is the behaviour the classic BBS already had.
func TestAPRSDisabledBlocksTransmission(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Op", Email: "o@example.invalid", Language: "en"})
	received := dbAPRSReceived{UserCallsign: "EA1ABC", Position: 0, At: now(), From: "DN9RZ", To: "EA1ABC", Text: "hi"}
	if err := s.db.Create(&received).Error; err != nil {
		t.Fatal(err)
	}

	form := invokeUserHandler(s.aprsSendForm, formRequest(http.MethodGet, "/aprs/send", nil), user)
	if form.Code != http.StatusOK {
		t.Fatalf("send form status = %d", form.Code)
	}
	if strings.Contains(form.Body.String(), `action="/aprs/send"`) {
		t.Fatal("the send form is still offered while APRS is disabled")
	}
	if !strings.Contains(form.Body.String(), "aprs_enable_required") && !strings.Contains(form.Body.String(), "not enabled") {
		t.Fatalf("no explanation for the disabled send form: %q", form.Body.String())
	}

	send := invokeUserHandler(s.aprsSend, formRequest(http.MethodPost, "/aprs/send", url.Values{
		"destination": {"EA7KLK"}, "destination_ssid": {"0"}, "text": {"should not go"},
	}), user)
	if send.Code == http.StatusSeeOther {
		t.Fatal("a POST to /aprs/send transmitted while APRS was disabled")
	}
	var sentCount int64
	s.db.Model(&dbAPRSSent{}).Where("user_callsign = ?", "EA1ABC").Count(&sentCount)
	if sentCount != 0 {
		t.Fatalf("a blocked send still wrote %d record(s)", sentCount)
	}

	reply := invokeUserHandler(s.aprsReceivedReply, setPathValue(formRequest(http.MethodPost, "/aprs/received/1/reply", url.Values{
		"destination": {"DN9RZ"}, "destination_ssid": {"0"}, "text": {"reply"},
	}), "id", "1"), user)
	if reply.Code == http.StatusSeeOther {
		t.Fatal("a reply transmitted while APRS was disabled")
	}
	s.db.Model(&dbAPRSSent{}).Where("user_callsign = ?", "EA1ABC").Count(&sentCount)
	if sentCount != 0 {
		t.Fatalf("a blocked reply still wrote %d record(s)", sentCount)
	}
}

// With APRS on, sending works again — the gate must not be a permanent block.
func TestAPRSEnabledAllowsSending(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Op", Email: "o@example.invalid", Language: "en", EnableAPRS: true})

	form := invokeUserHandler(s.aprsSendForm, formRequest(http.MethodGet, "/aprs/send", nil), user)
	if form.Code != http.StatusOK || !strings.Contains(form.Body.String(), `action="/aprs/send"`) {
		t.Fatalf("send form not offered with APRS enabled: %d", form.Code)
	}
	response := invokeUserHandler(s.aprsSend, formRequest(http.MethodPost, "/aprs/send", url.Values{
		"destination": {"EA7KLK"}, "destination_ssid": {"0"}, "text": {"hello"},
	}), user)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("send status = %d, want a redirect", response.Code)
	}
}

// The profile form is the other place the flag lives, and it must keep working.
func TestProfileFormStillSavesAPRSPreference(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{
		Callsign: "EA1ABC", FullName: "Op", Email: "o@example.invalid", Language: "en",
		PasswordHash: hashPassword("secret123"),
	})
	form := url.Values{
		"full_name":       {"Op"},
		"email":           {"o@example.invalid"},
		"language":        {"en"},
		"enable_aprs":     {"true"},
		"new_password":    {""},
		"verify_password": {""},
	}
	assertRedirect(t, invokeUserHandler(s.profilePost, formRequest(http.MethodPost, "/profile", form), user), "/profile?msg="+url.QueryEscape("web_profile_saved"))

	var saved dbUser
	if err := s.db.First(&saved, "callsign = ?", "EA1ABC").Error; err != nil {
		t.Fatal(err)
	}
	if !saved.EnableAPRS {
		t.Fatal("the profile form did not save the APRS preference")
	}
}

// Both surfaces must show the same answer, because they read the same column.
func TestAPRSSwitchMatchesTheStoredColumn(t *testing.T) {
	s := newTestServer(t)
	testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Op", Email: "o@example.invalid", EnableAPRS: true})

	// The SSH BBS writes this column directly; the page must follow it.
	if err := s.db.Model(&dbUser{}).Where("callsign = ?", "EA1ABC").Update("enable_aprs", false).Error; err != nil {
		t.Fatal(err)
	}
	body := invokeUserHandler(s.aprs, formRequest(http.MethodGet, "/aprs", nil), &dbUser{Callsign: "EA1ABC", FullName: "Op", Email: "o@example.invalid", Language: "en"}).Body.String()
	if !strings.Contains(body, "web_aprs_enable_action") {
		t.Fatal("the page did not follow the column: it still offers to disable")
	}
}

func TestAPRSToggleRequiresAuthentication(t *testing.T) {
	s := newTestServer(t)
	response := httptest.NewRecorder()
	s.routes().ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/aprs/toggle", strings.NewReader("enable_aprs=true")))
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login" {
		t.Fatalf("anonymous toggle = %d %q, want a redirect to /login", response.Code, response.Header().Get("Location"))
	}
}
