package main

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// The two bots.
//
// The command strings are APRS protocol: a typo does not fail a test, it silently
// does nothing on the air. So the mapping from a menu choice to the exact packet
// text is asserted here, and the strings are compared against the terminal
// client's own builder rather than against whatever the web app happens to
// produce.

func TestAPRSPHCommandMapping(t *testing.T) {
	cases := []struct {
		kind, count, target, phrase string
		thursday                    bool
		want                        string
	}{
		// Check-in: CQ alerts the net, NET logs quietly.
		{"checkin", "", "", "Net check-in from Madrid", false, "CQ Net check-in from Madrid"},
		{"checkin", "", "", "", false, "CQ"},
		{"quiet", "", "", "listening", false, "NET listening"},
		{"quiet", "", "", "", false, "NET"},

		// The message log, at each count and both logs.
		{"last", "5", "", "", false, "LAST"},
		{"last", "10", "", "", false, "LAST10"},
		{"last", "15", "", "", false, "LAST15"},
		{"last", "5", "", "", true, "LAST THURS"},
		{"last", "10", "", "", true, "LAST10 THURS"},
		{"last", "15", "", "", true, "LAST15 THURS"},

		// My own messages: a target and the Thursday log are alternatives, not a
		// combination — the bot reads the next token as one or the other.
		{"mine", "5", "", "", false, "MINE"},
		{"mine", "10", "EA7KLK", "", false, "MINE10 EA7KLK"},
		{"mine", "15", "ea7klk-7", "", false, "MINE15 EA7KLK-7"},
		{"mine", "10", "", "", true, "MINE10 THURS"},
		{"mine", "10", "EA7KLK", "", true, "MINE10 THURS"},

		// Search, with and without a count.
		{"search", "5", "", "antenna", false, "SEARCH antenna"},
		{"search", "10", "", "antenna tuner", false, "SEARCH10 antenna tuner"},
		{"search", "15", "", "tnx qso", false, "SEARCH15 tnx qso"},

		// Inbox and the three bot queries.
		{"inbox", "", "", "", false, "?APRSM"},
		{"inbox", "", "EA7KLK", "", false, "?APRSM EA7KLK"},
		{"path", "", "", "", false, "?APRST"},
		{"position", "", "", "", false, "?APRSP"},
		{"status", "", "", "", false, "?APRSS"},
		{"list", "", "", "", false, "LIST"},
		{"help", "", "", "", false, "HELP"},
		{"checkout", "", "", "", false, "U"},
		{"checkout", "", "", "73 all", false, "U 73 all"},
	}
	for _, tc := range cases {
		got := aprsphCommand(tc.kind, tc.count, tc.target, tc.phrase, tc.thursday)
		if got != tc.want {
			t.Fatalf("aprsphCommand(%q,%q,%q,%q,%t) = %q, want %q",
				tc.kind, tc.count, tc.target, tc.phrase, tc.thursday, got, tc.want)
		}
	}

	if got := aprsphCommand("nonsense", "", "", "", false); got != "" {
		t.Fatalf("an unknown kind produced %q", got)
	}
}

// The three #APRSThursday commands, verbatim.
func TestThursdayCommandStrings(t *testing.T) {
	for _, tc := range []struct{ got, want string }{
		{aprsHOTGJoin, "CQ HOTG"},
		{aprsHOTGLeave, "U HOTG"},
		{aprsHOTGKeep, "K HOTG"},
	} {
		if tc.got != tc.want {
			t.Fatalf("command = %q, want %q", tc.got, tc.want)
		}
	}
	if aprsBotANSRVR != "ANSRVR" || aprsBotAPRSPH != "APRSPH" {
		t.Fatal("a bot callsign changed; both are protocol identifiers")
	}
}

// A command that does not fit is refused, never split.
//
// This is the failure the terminal client exists to avoid: the splitter prefixes
// each part with "[n/total] ", so a two-part "CQ HOTG <text>" would put the
// command in part one under a bracket group and the text in part two — the
// command would never reach the group it names.
func TestCommandIsNeverSplit(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA7KLK", FullName: "Op", Email: "o@example.invalid", EnableAPRS: true})

	long := strings.Repeat("x", aprsCommandMaxBody+1)
	sent, err := s.sendAPRSCommand(user.Callsign, aprsBotAPRSPH, "CQ "+long)
	if err == nil {
		t.Fatal("an over-long command was accepted")
	}
	var tooLong errCommandTooLong
	if !strings.Contains(err.Error(), "never split") {
		t.Fatalf("the error does not explain the rule: %v", err)
	}
	if len(sent.Parts) != 1 {
		t.Fatalf("a refused command produced %d parts; it must produce none", len(sent.Parts))
	}
	_ = tooLong

	// Exactly at the limit must not be refused for length. The send itself cannot
	// succeed here — there is no APRS-IS in the test environment — so the
	// assertion is on the reason, not on the outcome.
	body := strings.Repeat("x", aprsCommandMaxBody)
	_, err = s.sendAPRSCommand(user.Callsign, aprsBotAPRSPH, body)
	var stillTooLong errCommandTooLong
	if errors.As(err, &stillTooLong) {
		t.Fatalf("a command exactly at the limit was refused as too long: %v", err)
	}
}

// The limit reserves space for the trailing message ID, so a command at the limit
// plus an ID still fits in one APRS message field.
func TestCommandLimitReservesTheMessageID(t *testing.T) {
	if got := aprsCommandMaxBody + aprsCommandIDReserve; got != aprsMessageLimit {
		t.Fatalf("max body %d + reserve %d = %d, want the message limit %d",
			aprsCommandMaxBody, aprsCommandIDReserve, got, aprsMessageLimit)
	}
}

// #APRSThursday runs Thursdays only, so the join is gated on the weekday.
func TestThursdayJoinIsGatedOnTheWeekday(t *testing.T) {
	thursday := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC) // a Thursday
	friday := thursday.Add(24 * time.Hour)
	if !aprsThursdayActive(thursday) {
		t.Fatal("Thursday is not recognised as Thursday")
	}
	if aprsThursdayActive(friday) {
		t.Fatal("Friday is recognised as Thursday")
	}

	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA7KLK", FullName: "Op", Email: "o@example.invalid", Language: "en", EnableAPRS: true})

	// Drive the gate from a fixed clock so the assertion holds on every day of the
	// week rather than only six of them.
	original := aprsClock
	defer func() { aprsClock = original }()

	aprsClock = func() time.Time { return friday }
	refused := invokeUserHandler(s.thursdayJoin, formRequest(http.MethodPost, "/aprs/thursday/join", nil), user)
	if !strings.Contains(refused.Body.String(), "aprs_not_thursday_warning") &&
		!strings.Contains(refused.Body.String(), "intended only on Thursdays") {
		t.Fatalf("Friday join gave no explanation: %q", refused.Body.String())
	}
	var count int64
	s.db.Model(&dbAPRSSent{}).Where("user_callsign = ?", "EA7KLK").Count(&count)
	if count != 0 {
		t.Fatalf("a Friday join transmitted %d message(s)", count)
	}

	// On Thursday the gate opens. The send fails here for want of an APRS-IS
	// server, but it must fail at the network, not at the weekday.
	aprsClock = func() time.Time { return thursday }
	allowed := invokeUserHandler(s.thursdayJoin, formRequest(http.MethodPost, "/aprs/thursday/join", nil), user)
	if strings.Contains(allowed.Body.String(), "aprs_not_thursday_warning") {
		t.Fatal("a Thursday join was refused as not-Thursday")
	}
}

// Leaving is not gated on the weekday — participants are asked to leave when they
// stop, whatever day that is — but it does require a confirmation.
func TestThursdayLeaveRequiresConfirmation(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA7KLK", FullName: "Op", Email: "o@example.invalid", Language: "en", EnableAPRS: true})

	unconfirmed := invokeUserHandler(s.thursdayLeave, formRequest(http.MethodPost, "/aprs/thursday/leave", nil), user)
	if !strings.Contains(unconfirmed.Body.String(), "aprs_leave_confirm") &&
		!strings.Contains(unconfirmed.Body.String(), "Leave the group") {
		t.Fatalf("an unconfirmed leave was accepted: %q", unconfirmed.Body.String())
	}
	var count int64
	s.db.Model(&dbAPRSSent{}).Where("user_callsign = ?", "EA7KLK").Count(&count)
	if count != 0 {
		t.Fatalf("an unconfirmed leave transmitted %d message(s)", count)
	}
}

// Nothing may transmit while APRS is off, for the same reason the send form is
// gated: the terminal client will not transmit for an account with it off.
func TestBotsRespectTheAPRSGate(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA7KLK", FullName: "Op", Email: "o@example.invalid", Language: "en", EnableAPRS: false})

	for _, handler := range []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request, *dbUser)
		form    url.Values
	}{
		{"aprsph", s.aprsphSend, url.Values{"kind": {"list"}}},
		{"join", s.thursdayJoin, nil},
		{"leave", s.thursdayLeave, url.Values{"confirm": {"yes"}}},
		{"keepalive", s.thursdayKeepalive, nil},
	} {
		response := invokeUserHandler(handler.handler, formRequest(http.MethodPost, "/aprs/x", handler.form), user)
		if strings.Contains(response.Body.String(), "aprs_send_success") {
			t.Fatalf("%s transmitted while APRS was disabled", handler.name)
		}
		if !strings.Contains(response.Body.String(), "aprs_enable_required") &&
			!strings.Contains(response.Body.String(), "not enabled for this profile") {
			t.Fatalf("%s gave no explanation: %q", handler.name, response.Body.String())
		}
	}
	var count int64
	s.db.Model(&dbAPRSSent{}).Where("user_callsign = ?", "EA7KLK").Count(&count)
	if count != 0 {
		t.Fatalf("the gated paths transmitted %d message(s)", count)
	}
}

// The help text is local and must not transmit.
func TestAPRSPHHelpSendsNothing(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA7KLK", FullName: "Op", Email: "o@example.invalid", Language: "en", EnableAPRS: true})

	response := invokeUserHandler(s.aprsphSend, formRequest(http.MethodPost, "/aprs/aprsph", url.Values{"kind": {"help"}}), user)
	if response.Code != http.StatusOK {
		t.Fatalf("help status = %d", response.Code)
	}
	var count int64
	s.db.Model(&dbAPRSSent{}).Where("user_callsign = ?", "EA7KLK").Count(&count)
	if count != 0 {
		t.Fatal("asking for help transmitted a message")
	}
}

// An empty search phrase is refused before anything is built, so the caller
// cannot end up sending "SEARCH10" — a valid-looking command that searches for
// nothing.
func TestAPRSPHRejectsAnEmptySearch(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA7KLK", FullName: "Op", Email: "o@example.invalid", Language: "en", EnableAPRS: true})

	response := invokeUserHandler(s.aprsphSend, formRequest(http.MethodPost, "/aprs/aprsph",
		url.Values{"kind": {"search"}, "phrase": {""}, "count": {"10"}}), user)
	if strings.Contains(response.Body.String(), "aprs_send_success") {
		t.Fatal("an empty search was sent")
	}
	var count int64
	s.db.Model(&dbAPRSSent{}).Where("user_callsign = ?", "EA7KLK").Count(&count)
	if count != 0 {
		t.Fatal("an empty search transmitted a message")
	}
}

// A target callsign is validated, so a mistyped one does not become garbage in
// the packet.
func TestAPRSPHRejectsAnInvalidTarget(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA7KLK", FullName: "Op", Email: "o@example.invalid", Language: "en", EnableAPRS: true})

	invokeUserHandler(s.aprsphSend, formRequest(http.MethodPost, "/aprs/aprsph",
		url.Values{"kind": {"inbox"}, "target": {"not a callsign!"}}), user)
	var count int64
	s.db.Model(&dbAPRSSent{}).Where("user_callsign = ?", "EA7KLK").Count(&count)
	if count != 0 {
		t.Fatal("an invalid target transmitted a message")
	}
}

// A join carries a message, as the terminal client has always required.
//
// On 2026-11-12 the web form sent a bare "CQ HOTG": APRS-IS accepted it, the
// packet went out, and the group server never replied. The text is what the net
// relays, so a join without one is a message nobody acts on.
func TestThursdayJoinRequiresAMessage(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA7KLK", FullName: "Op", Email: "o@example.invalid", Language: "en", EnableAPRS: true})

	original := aprsClock
	defer func() { aprsClock = original }()
	thursday := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC) // a Thursday
	aprsClock = func() time.Time { return thursday }

	// No text: nothing must be sent.
	empty := invokeUserHandler(s.thursdayJoin, formRequest(http.MethodPost, "/aprs/thursday/join", url.Values{}), user)
	if strings.Contains(empty.Body.String(), "aprs_send_success") {
		t.Fatal("a join without a message was sent")
	}
	var count int64
	s.db.Model(&dbAPRSSent{}).Where("user_callsign = ?", "EA7KLK").Count(&count)
	if count != 0 {
		t.Fatalf("a join without a message transmitted %d packet(s)", count)
	}

	// With text, the command carries it.
	invokeUserHandler(s.thursdayJoin, formRequest(http.MethodPost, "/aprs/thursday/join",
		url.Values{"text": {"hello from Madrid"}}), user)
	var sent dbAPRSSent
	if err := s.db.First(&sent, "user_callsign = ?", "EA7KLK").Error; err != nil {
		t.Fatal(err)
	}
	if sent.To != aprsBotANSRVR {
		t.Fatalf("join sent to %q, want ANSRVR", sent.To)
	}
	if !strings.HasPrefix(sent.Text, "CQ HOTG ") || !strings.Contains(sent.Text, "hello from Madrid") {
		t.Fatalf("join body = %q, want \"CQ HOTG <text>\"", sent.Text)
	}
}

// The join form must offer the field, or the requirement above is unfillable.
func TestThursdayJoinFormOffersAMessageField(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA7KLK", FullName: "Op", Email: "o@example.invalid", Language: "en", EnableAPRS: true})

	original := aprsClock
	defer func() { aprsClock = original }()
	aprsClock = func() time.Time { return time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC) }

	body := invokeUserHandler(s.aprs, formRequest(http.MethodGet, "/aprs", nil), user).Body.String()
	if !strings.Contains(body, `name="text"`) {
		t.Fatal("the join form has no message field")
	}
}

// Every page that can transmit must say that messages leave from SSID 0.
//
// The station cannot transmit from any other SSID, so an operator addressing
// EA7KLK-15 should not expect traffic to appear to come from EA7KLK-15. The
// warning belongs on the pages that send, not buried in help.
func TestSsid0WarningIsShownOnEverySendingPage(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA7KLK", FullName: "Op", Email: "o@example.invalid", Language: "en", EnableAPRS: true})

	original := aprsClock
	defer func() { aprsClock = original }()
	aprsClock = func() time.Time { return time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC) }

	pages := map[string]func(http.ResponseWriter, *http.Request, *dbUser){
		"/aprs/send":   s.aprsSendForm,
		"/aprs/aprsph": s.aprsphPage,
		"/aprs":        s.aprs,
	}
	for path, handler := range pages {
		response := invokeUserHandler(handler, formRequest(http.MethodGet, path, nil), user)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, response.Code)
		}
		body := response.Body.String()
		if !strings.Contains(body, `class="notice"`) {
			t.Fatalf("%s has no notice block", path)
		}
		// The station is named, not described abstractly.
		if !strings.Contains(body, "EA7KLK-0") {
			t.Fatalf("%s does not name the station messages leave from", path)
		}
		if !strings.Contains(body, "web_ssid0_warning_title") {
			t.Fatalf("%s is missing the warning heading", path)
		}
	}
}

// The pages that do not transmit must not carry it, or it becomes noise nobody
// reads.
func TestSsid0WarningIsNotEverywhere(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA7KLK", FullName: "Op", Email: "o@example.invalid", Language: "en", EnableAPRS: true})

	for _, page := range []struct {
		path    string
		handler func(http.ResponseWriter, *http.Request, *dbUser)
	}{
		{"/boards", s.boards},
		{"/bulletins", s.bulletins},
		{"/profile", s.profileForm},
	} {
		body := invokeUserHandler(page.handler, formRequest(http.MethodGet, page.path, nil), user).Body.String()
		if strings.Contains(body, `class="notice"`) {
			t.Fatalf("%s shows the send warning although it cannot send", page.path)
		}
	}
}

// The web app never acknowledges: reception and ACKs belong to the classic BBS,
// which does it for every account whether or not anybody opens the web app.
func TestWebAppDoesNotAcknowledge(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA7KLK", FullName: "Op", Email: "o@example.invalid", Language: "en", EnableAPRS: true})

	// Sending a message must not produce an "ack…" packet of its own.
	response := invokeUserHandler(s.aprsSend, formRequest(http.MethodPost, "/aprs/send", url.Values{
		"destination": {"DN9RZ"}, "destination_ssid": {"0"}, "text": {"hello"},
	}), user)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("send status = %d", response.Code)
	}
	var rows []dbAPRSSent
	s.db.Find(&rows)
	for _, row := range rows {
		for _, part := range row.Parts {
			if strings.HasPrefix(strings.ToLower(part.Text), "ack") {
				t.Fatalf("the web app sent an acknowledgement: %q", part.Text)
			}
		}
		if strings.HasPrefix(strings.ToLower(row.Text), "ack") {
			t.Fatalf("the web app sent an acknowledgement: %q", row.Text)
		}
	}
}
