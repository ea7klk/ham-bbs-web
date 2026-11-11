package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// testMailer returns a mailer that records what it would have sent instead of
// talking to an SMTP server, with a configuration that counts as enabled.
func testMailer() (*mailer, *[]mailMessage) {
	sent := []mailMessage{}
	m := &mailer{
		cfg: mailConfig{
			host:       "mail.example.invalid",
			port:       587,
			username:   "bbs@example.invalid",
			password:   "a-real-looking-password",
			from:       "bbs@example.invalid",
			fromName:   "Test BBS",
			security:   "starttls",
			webBaseURL: "https://bbs.example.invalid",
		},
		now: time.Now,
	}
	m.send = func(msg mailMessage) error {
		sent = append(sent, msg)
		return nil
	}
	return m, &sent
}

// tokenFromMail pulls the token out of the link in a captured message, which is
// the same thing a user would click.
func tokenFromMail(t *testing.T, body string) string {
	t.Helper()
	field := strings.Fields(body)
	for _, word := range field {
		if i := strings.Index(word, "token="); i >= 0 {
			return strings.TrimRight(word[i+len("token="):], ".,)")
		}
	}
	t.Fatalf("no token in mail body: %q", body)
	return ""
}

func TestActivationCodeFlow(t *testing.T) {
	s := newTestServer(t)
	m, sent := testMailer()
	s.mail = m

	profile := testUser(t, s, dbUser{
		Callsign: "EA1ABC",
		FullName: "Operator",
		Email:    "op@example.invalid",
		Language: "en",
	})
	if err := s.db.Model(&dbUser{}).Where("callsign = ?", profile.Callsign).Update("activated", false).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.sendActivationCode(*profile); err != nil {
		t.Fatal(err)
	}
	if len(*sent) != 1 {
		t.Fatalf("activation sent %d messages, want 1", len(*sent))
	}
	// The code is not in the database: only its hash is, so the mail is the one
	// place to read it from.
	mail := (*sent)[0]
	var code string
	for _, line := range strings.Split(mail.Body, "\n") {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) == activationCodeLength && strings.Trim(trimmed, activationAlphabet) == "" {
			code = trimmed
		}
	}
	if code == "" {
		t.Fatalf("no activation code in %q", mail.Body)
	}
	var stored dbActivation
	if err := s.db.First(&stored, "callsign = ?", "EA1ABC").Error; err != nil {
		t.Fatal(err)
	}
	if stored.CodeHash == code {
		t.Fatal("the activation code is stored in plaintext")
	}

	// A wrong code is refused and counted.
	if err := s.verifyActivationCode("EA1ABC", "ZZZZ", time.Now()); err == nil {
		t.Fatal("a wrong activation code was accepted")
	}
	var afterFail dbActivation
	if err := s.db.First(&afterFail, "callsign = ?", "EA1ABC").Error; err != nil {
		t.Fatal(err)
	}
	if afterFail.Attempts != 1 {
		t.Fatalf("failed attempt was not counted: attempts=%d", afterFail.Attempts)
	}

	// The right code clears the pending row, and the caller activates the account.
	if err := s.verifyActivationCode("EA1ABC", code, time.Now()); err != nil {
		t.Fatalf("correct code rejected: %v", err)
	}
	if err := s.verifyActivationCode("EA1ABC", code, time.Now()); err == nil {
		t.Fatal("a consumed code was accepted twice")
	}
}

func TestActivationRejectsExpiredAndExhaustedCodes(t *testing.T) {
	s := newTestServer(t)
	if _, err := s.issueActivationCode("EA1ABC", time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := s.verifyActivationCode("EA1ABC", "ZZZZ", time.Now().Add(activationCodeTTL+time.Minute)); err == nil {
		t.Fatal("an expired code was accepted")
	}

	if _, err := s.issueActivationCode("EA2XYZ", time.Now()); err != nil {
		t.Fatal(err)
	}
	for attempt := 0; attempt < activationAttempts; attempt++ {
		_ = s.verifyActivationCode("EA2XYZ", "ZZZZ", time.Now())
	}
	if err := s.verifyActivationCode("EA2XYZ", "ZZZZ", time.Now()); err != errActivationTooMany {
		t.Fatalf("exhausted code error = %v, want errActivationTooMany", err)
	}
}

func TestRegistrationSendsACodeAndGatesLogin(t *testing.T) {
	s := newTestServer(t)
	m, sent := testMailer()
	s.mail = m

	form := url.Values{
		"callsign":        {"EA1ABC"},
		"full_name":       {"Operator"},
		"email":           {"op@example.invalid"},
		"language":        {"en"},
		"new_password":    {"secret123"},
		"verify_password": {"secret123"},
	}
	response := invokeHandler(s.registerPost, formRequest(http.MethodPost, "/register", form))
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/activate?callsign=EA1ABC" {
		t.Fatalf("registration = %d %q, want a redirect to the activation form", response.Code, response.Header().Get("Location"))
	}
	if len(*sent) != 1 {
		t.Fatalf("registration sent %d messages, want 1", len(*sent))
	}
	var stored dbUser
	if err := s.db.First(&stored, "callsign = ?", "EA1ABC").Error; err != nil {
		t.Fatal(err)
	}
	if stored.Activated {
		t.Fatal("a registrant was marked active before entering the code")
	}

	// A correct password on an unactivated account does not create a session.
	login := invokeHandler(s.loginPost, formRequest(http.MethodPost, "/login", url.Values{"callsign": {"EA1ABC"}, "password": {"secret123"}}))
	if login.Code != http.StatusSeeOther || login.Header().Get("Location") != "/activate?callsign=EA1ABC" {
		t.Fatalf("unactivated login = %d %q", login.Code, login.Header().Get("Location"))
	}
	if len(s.sessions) != 0 {
		t.Fatalf("an unactivated account was given a session")
	}

	// Entering the code activates the account, after which login works.
	code := ""
	for _, line := range strings.Split((*sent)[0].Body, "\n") {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) == activationCodeLength && strings.Trim(trimmed, activationAlphabet) == "" {
			code = trimmed
		}
	}
	activate := invokeHandler(s.public(s.activatePost), formRequest(http.MethodPost, "/activate", url.Values{
		"callsign": {"EA1ABC"}, "code": {code}, "action": {"activate"},
	}))
	if activate.Code != http.StatusOK || !strings.Contains(activate.Body.String(), "web_activated_ok") {
		t.Fatalf("activation response = %d %q", activate.Code, activate.Body.String())
	}
	if err := s.db.First(&stored, "callsign = ?", "EA1ABC").Error; err != nil {
		t.Fatal(err)
	}
	if !stored.Activated {
		t.Fatal("the account was not marked active after the correct code")
	}
	assertRedirect(t, invokeHandler(s.loginPost, formRequest(http.MethodPost, "/login", url.Values{"callsign": {"EA1ABC"}, "password": {"secret123"}})), "/")
}

func TestRegistrationIsThrottledPerAddress(t *testing.T) {
	s := newTestServer(t)
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	address := "203.0.113.7"

	for i := 0; i < registrationLimit; i++ {
		allowed, err := s.registrationAllowed(address, now)
		if err != nil {
			t.Fatal(err)
		}
		if !allowed {
			t.Fatalf("address blocked at registration %d of %d", i+1, registrationLimit)
		}
		if err := s.recordRegistration(address, now); err != nil {
			t.Fatal(err)
		}
	}
	allowed, err := s.registrationAllowed(address, now)
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatalf("address was allowed past the limit of %d per window", registrationLimit)
	}

	// Another address is unaffected, and a new window resets the count.
	if other, _ := s.registrationAllowed("198.51.100.9", now); !other {
		t.Fatal("one address's limit blocked another address")
	}
	later := now.Add(registrationWindow + time.Minute)
	if allowed, _ := s.registrationAllowed(address, later); !allowed {
		t.Fatal("the limit did not reset after the window")
	}
}

func TestPasswordResetFlow(t *testing.T) {
	s := newTestServer(t)
	m, sent := testMailer()
	s.mail = m
	testUser(t, s, dbUser{
		Callsign:     "EA1ABC",
		FullName:     "Operator",
		Email:        "op@example.invalid",
		Language:     "en",
		PasswordHash: hashPassword("oldsecret"),
	})

	// The handler renders the result page rather than redirecting, so the
	// message is checked on the body. The fixture translation map is empty, so
	// the key is what comes back.
	response := invokeHandler(s.public(s.forgotPost), formRequest(http.MethodPost, "/forgot", url.Values{"identifier": {"EA1ABC"}}))
	if !strings.Contains(response.Body.String(), "forgot_password_sent") {
		t.Fatalf("reset request response = %q", response.Body.String())
	}
	if len(*sent) < 1 {
		t.Fatal("no reset mail was sent")
	}
	token := tokenFromMail(t, (*sent)[0].Body)

	// A GET shows the form and does not spend the link, so a prefetch or a
	// reload cannot burn it.
	for i := 0; i < 2; i++ {
		form := invokeHandler(s.public(s.resetForm), formRequest(http.MethodGet, "/reset?token="+token, nil))
		if form.Code != http.StatusOK || !strings.Contains(form.Body.String(), token) {
			t.Fatalf("reset form %d = %d %q", i+1, form.Code, form.Body.String())
		}
	}

	// A short password is refused before the token is spent.
	short := invokeHandler(s.public(s.resetPost), formRequest(http.MethodPost, "/reset", url.Values{
		"token": {token}, "new_password": {"short"}, "verify_password": {"short"},
	}))
	if !strings.Contains(short.Body.String(), "password_short") {
		t.Fatalf("short password response = %q", short.Body.String())
	}

	ok := invokeHandler(s.public(s.resetPost), formRequest(http.MethodPost, "/reset", url.Values{
		"token": {token}, "new_password": {"newsecret1"}, "verify_password": {"newsecret1"},
	}))
	if ok.Code != http.StatusOK || !strings.Contains(ok.Body.String(), "web_password_reset") {
		t.Fatalf("reset response = %d %q", ok.Code, ok.Body.String())
	}
	var stored dbUser
	if err := s.db.First(&stored, "callsign = ?", "EA1ABC").Error; err != nil {
		t.Fatal(err)
	}
	if !verifyPassword("newsecret1", stored.PasswordHash) {
		t.Fatal("the new password was not stored")
	}
	if verifyPassword("oldsecret", stored.PasswordHash) {
		t.Fatal("the old password still works")
	}

	// The link is single use.
	replay := invokeHandler(s.public(s.resetPost), formRequest(http.MethodPost, "/reset", url.Values{
		"token": {token}, "new_password": {"another12"}, "verify_password": {"another12"},
	}))
	if !strings.Contains(replay.Body.String(), "web_token_invalid") {
		t.Fatalf("replayed reset was accepted: %q", replay.Body.String())
	}
}

func TestPasswordResetIsQuietAboutUnknownAccounts(t *testing.T) {
	s := newTestServer(t)
	m, sent := testMailer()
	s.mail = m

	known := invokeHandler(s.public(s.forgotPost), formRequest(http.MethodPost, "/forgot", url.Values{"identifier": {"NOBODY"}}))
	if !strings.Contains(known.Body.String(), "forgot_password_sent") {
		t.Fatalf("unknown account response = %q", known.Body.String())
	}
	if len(*sent) != 0 {
		t.Fatalf("a reset mail was sent for an unknown account")
	}
}

func TestPasswordResetQuotaIsPerAccount(t *testing.T) {
	s := newTestServer(t)
	m, _ := testMailer()
	s.mail = m
	testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Op", Email: "op@example.invalid", PasswordHash: hashPassword("oldsecret")})

	for i := 0; i < resetRequestsPerHour; i++ {
		if err := s.sendPasswordReset("EA1ABC"); err != nil {
			t.Fatalf("reset %d: %v", i+1, err)
		}
	}
	if err := s.sendPasswordReset("EA1ABC"); err == nil {
		t.Fatal("the reset quota was not enforced")
	}
}

func TestEmailChangeNeedsBothConfirmations(t *testing.T) {
	s := newTestServer(t)
	m, sent := testMailer()
	s.mail = m
	testUser(t, s, dbUser{
		Callsign: "EA1ABC",
		FullName: "Operator",
		Email:    "old@example.invalid",
		Language: "en",
	})

	form := url.Values{
		"full_name":       {"Operator"},
		"email":           {"new@example.invalid"},
		"language":        {"en"},
		"new_password":    {""},
		"verify_password": {""},
	}
	profile := invokeUserHandler(s.profilePost, formRequest(http.MethodPost, "/profile", form), &dbUser{
		Callsign: "EA1ABC", FullName: "Operator", Email: "old@example.invalid", Language: "en", Activated: true,
	})
	// The fixture translation map is empty, so the message key is what comes back
	// from sprintfAll: the point of the assertion is that the pending-change
	// message was chosen, not the wording.
	assertRedirect(t, profile, "/profile?msg="+url.QueryEscape("email_change_pending_detail"))

	// The address has not moved yet.
	var stored dbUser
	if err := s.db.First(&stored, "callsign = ?", "EA1ABC").Error; err != nil {
		t.Fatal(err)
	}
	if stored.Email != "old@example.invalid" {
		t.Fatalf("the address changed before both confirmations: %q", stored.Email)
	}
	if len(*sent) != 2 {
		t.Fatalf("email change sent %d messages, want 2", len(*sent))
	}
	oldToken := tokenFromMail(t, (*sent)[0].Body)
	newToken := tokenFromMail(t, (*sent)[1].Body)

	// One half only.
	half := invokeHandler(s.public(s.emailConfirm), formRequest(http.MethodGet, "/email/confirm?token="+oldToken, nil))
	if !strings.Contains(half.Body.String(), "web_email_change_half") {
		t.Fatalf("half confirmation response = %q", half.Body.String())
	}
	if err := s.db.First(&stored, "callsign = ?", "EA1ABC").Error; err != nil {
		t.Fatal(err)
	}
	if stored.Email != "old@example.invalid" {
		t.Fatalf("one confirmation applied the change: %q", stored.Email)
	}

	// Both halves.
	both := invokeHandler(s.public(s.emailConfirm), formRequest(http.MethodGet, "/email/confirm?token="+newToken, nil))
	if !strings.Contains(both.Body.String(), "web_email_change_applied") {
		t.Fatalf("second confirmation response = %q", both.Body.String())
	}
	if err := s.db.First(&stored, "callsign = ?", "EA1ABC").Error; err != nil {
		t.Fatal(err)
	}
	if stored.Email != "new@example.invalid" {
		t.Fatalf("the change was not applied after both confirmations: %q", stored.Email)
	}
}

// Without SMTP there is nothing to confirm with, so the change is applied and the
// log records that it was not verified — the same trade-off the SSH BBS makes.
func TestEmailChangeAppliesWithoutMail(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Operator", Email: "old@example.invalid", Language: "en"})
	form := url.Values{
		"full_name":       {"Operator"},
		"email":           {"new@example.invalid"},
		"language":        {"en"},
		"new_password":    {""},
		"verify_password": {""},
	}
	assertRedirect(t, invokeUserHandler(s.profilePost, formRequest(http.MethodPost, "/profile", form), user), "/profile?msg="+url.QueryEscape("web_profile_saved"))
	var stored dbUser
	if err := s.db.First(&stored, "callsign = ?", "EA1ABC").Error; err != nil {
		t.Fatal(err)
	}
	if stored.Email != "new@example.invalid" {
		t.Fatalf("the change was not applied without mail: %q", stored.Email)
	}
}

// A token written the way the SSH BBS writes it — hash of the plaintext, in the
// shared table — must be redeemable here, because that is how a reset link
// mailed by the terminal lands in a browser.
func TestTokenIssuedByTheSSHBBSIsRedeemable(t *testing.T) {
	s := newTestServer(t)
	testUser(t, s, dbUser{
		Callsign:     "EA1ABC",
		FullName:     "Operator",
		Email:        "op@example.invalid",
		Language:     "en",
		PasswordHash: hashPassword("oldsecret"),
	})

	raw := "a-token-the-ssh-bbs-would-have-generated"
	row := dbToken{
		TokenHash: hashToken(raw),
		Purpose:   tokenPurposePasswordReset,
		Callsign:  "EA1ABC",
		Email:     "op@example.invalid",
		CreatedAt: time.Now().Unix(),
		ExpiresAt: time.Now().Add(passwordResetTTL).Unix(),
	}
	if err := s.db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}

	form := invokeHandler(s.public(s.resetForm), formRequest(http.MethodGet, "/reset?token="+raw, nil))
	if form.Code != http.StatusOK || !strings.Contains(form.Body.String(), "EA1ABC") {
		t.Fatalf("reset form for a foreign token = %d %q", form.Code, form.Body.String())
	}
	ok := invokeHandler(s.public(s.resetPost), formRequest(http.MethodPost, "/reset", url.Values{
		"token": {raw}, "new_password": {"newsecret1"}, "verify_password": {"newsecret1"},
	}))
	if !strings.Contains(ok.Body.String(), "web_password_reset") {
		t.Fatalf("foreign token was not redeemed: %q", ok.Body.String())
	}
}

func TestPasswordMinimumLengthIsShared(t *testing.T) {
	s := newTestServer(t)
	form := validUserForm()
	form.Set("new_password", "short")
	form.Set("verify_password", "short")
	_, errText := s.userFromForm(formRequest(http.MethodPost, "/register", form), dbUser{}, true)
	if errText != "password_short" {
		t.Fatalf("short password error = %q, want password_short", errText)
	}
}

func TestStaticParityPagesRender(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Operator", Email: "op@example.invalid"})

	for _, page := range []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request, *dbUser)
		path    string
		key     string
	}{
		{"about", s.about, "/about", "about_title"},
		{"resources", s.resources, "/resources", "resources_title"},
	} {
		response := invokeUserHandler(page.handler, formRequest(http.MethodGet, page.path, nil), user)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d", page.name, response.Code)
		}
		if !strings.Contains(response.Body.String(), page.key) {
			t.Fatalf("%s did not render %q", page.name, page.key)
		}
	}
}

func TestSysopCanEditAMessage(t *testing.T) {
	s := newTestServer(t)
	seedBoard(t, s)
	user := testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Operator", Email: "op@example.invalid", IsSysop: true})
	id, err := s.createMessage(dbMessage{BoardID: defaultBoardID, From: "EA1ABC", Subject: "Tpyo", Body: "Bdoy"})
	if err != nil {
		t.Fatal(err)
	}

	form := invokeUserHandler(s.messageEditForm, setPathValue(formRequest(http.MethodGet, "/messages/1/edit", nil), "id", "1"), user)
	if form.Code != http.StatusOK || !strings.Contains(form.Body.String(), "Tpyo") {
		t.Fatalf("edit form = %d %q", form.Code, form.Body.String())
	}

	response := invokeUserHandler(s.messageUpdate, setPathValue(formRequest(http.MethodPost, "/messages/1/edit", url.Values{
		"subject": {"Typo"}, "body": {"Body"},
	}), "id", "1"), user)
	assertRedirect(t, response, "/boards/general?msg="+url.QueryEscape("message_updated"))

	var stored dbMessage
	if err := s.db.First(&stored, id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Subject != "Typo" || stored.Body != "Body" {
		t.Fatalf("the edit was not stored: %#v", stored)
	}
	if stored.Edited == "" {
		t.Fatal("the edit timestamp was not recorded")
	}
}

func TestMessageEditSanitisesAndRejectsEmpty(t *testing.T) {
	s := newTestServer(t)
	seedBoard(t, s)
	user := testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Operator", Email: "op@example.invalid", IsSysop: true})
	id, err := s.createMessage(dbMessage{BoardID: defaultBoardID, From: "EA1ABC", Subject: "Root", Body: "Body"})
	if err != nil {
		t.Fatal(err)
	}

	empty := invokeUserHandler(s.messageUpdate, setPathValue(formRequest(http.MethodPost, "/messages/1/edit", url.Values{
		"subject": {" "}, "body": {"Body"},
	}), "id", "1"), user)
	if empty.Code != http.StatusOK || !strings.Contains(empty.Body.String(), "required") {
		t.Fatalf("empty edit = %d %q", empty.Code, empty.Body.String())
	}

	invokeUserHandler(s.messageUpdate, setPathValue(formRequest(http.MethodPost, "/messages/1/edit", url.Values{
		"subject": {"hi\x1b[2Jthere"}, "body": {"a\x1b]52;c;eA==\x07b"},
	}), "id", "1"), user)
	var stored dbMessage
	if err := s.db.First(&stored, id).Error; err != nil {
		t.Fatal(err)
	}
	if strings.ContainsRune(stored.Subject, '\x1b') || strings.ContainsRune(stored.Body, '\x1b') {
		t.Fatalf("the edit kept a terminal sequence: %#v", stored)
	}
}
