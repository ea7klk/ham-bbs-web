package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func validUserForm() url.Values {
	return url.Values{
		"callsign":        {"ea7klk"},
		"full_name":       {" Test Operator "},
		"email":           {"operator@example.invalid"},
		"maidenhead":      {"im77ah"},
		"language":        {"es"},
		"enable_aprs":     {"true"},
		"qth":             {" Sevilla "},
		"rig":             {" VHF "},
		"new_password":    {"secret"},
		"verify_password": {"secret"},
	}
}

func TestUserFromFormValidation(t *testing.T) {
	s := newTestServer(t)
	form := validUserForm()
	user, errText := s.userFromForm(formRequest(http.MethodPost, "/register", form), dbUser{}, true)
	if errText != "" {
		t.Fatalf("valid user form error = %q", errText)
	}
	if user.Callsign != "EA7KLK" || user.FullName != "Test Operator" || user.Maidenhead != "IM77ah" || user.Language != "es" || !user.EnableAPRS || user.QTH != "Sevilla" || user.Rig != "VHF" {
		t.Fatalf("parsed user = %#v", user)
	}
	if !verifyPassword("secret", user.PasswordHash) {
		t.Fatal("parsed password was not hashed correctly")
	}

	tests := []struct {
		name string
		form url.Values
		want string
	}{
		{"invalid callsign", func() url.Values { v := validUserForm(); v.Set("callsign", "x"); return v }(), "web_invalid_callsign"},
		{"required profile", func() url.Values { v := validUserForm(); v.Set("full_name", ""); return v }(), "web_required_profile"},
		{"invalid language", func() url.Values { v := validUserForm(); v.Set("language", "xx"); return v }(), "web_invalid_language"},
		{"invalid locator", func() url.Values { v := validUserForm(); v.Set("maidenhead", "ZZ99"); return v }(), "web_invalid_locator"},
		{"password mismatch", func() url.Values { v := validUserForm(); v.Set("verify_password", "different"); return v }(), "web_password_match"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, got := s.userFromForm(formRequest(http.MethodPost, "/register", test.form), dbUser{}, true)
			if got != test.want {
				t.Fatalf("userFromForm() error = %q, want %q", got, test.want)
			}
		})
	}
	profile := validUserForm()
	profile.Del("new_password")
	profile.Del("verify_password")
	profile.Del("callsign")
	updated, errText := s.userFromForm(formRequest(http.MethodPost, "/profile", profile), dbUser{Callsign: "EA7KLK", PasswordHash: "keep"}, false)
	if errText != "" || updated.PasswordHash != "keep" {
		t.Fatalf("profile form without password = %#v, error=%q", updated, errText)
	}
}

func TestRegisterLoginAndProfileHandlers(t *testing.T) {
	s := newTestServer(t)
	s.cfg.sysops["EA7KLK"] = true
	form := validUserForm()
	response := invokeHandler(s.registerPost, formRequest(http.MethodPost, "/register", form))
	assertRedirect(t, response, "/")
	var saved dbUser
	if err := s.db.First(&saved, "callsign = ?", "EA7KLK").Error; err != nil {
		t.Fatal(err)
	}
	if !saved.IsSysop || saved.FirstSeen == "" || saved.LastSeen == "" {
		t.Fatalf("registered user metadata = %#v", saved)
	}
	if len(s.sessions) != 1 || response.Header().Get("Set-Cookie") == "" {
		t.Fatal("registration did not create a session")
	}

	duplicate := invokeHandler(s.registerPost, formRequest(http.MethodPost, "/register", form))
	if duplicate.Code != http.StatusOK || !strings.Contains(duplicate.Body.String(), "web_duplicate_callsign") {
		t.Fatalf("duplicate registration response = %d %q", duplicate.Code, duplicate.Body.String())
	}

	unknown := invokeHandler(s.loginPost, formRequest(http.MethodPost, "/login", url.Values{"callsign": {"noone"}, "password": {"secret"}}))
	assertRedirect(t, unknown, "/register?callsign=NOONE")

	saved.Disabled = true
	s.db.Save(&saved)
	disabled := invokeHandler(s.loginPost, formRequest(http.MethodPost, "/login", url.Values{"callsign": {"EA7KLK"}, "password": {"secret"}}))
	if disabled.Code != http.StatusOK || !strings.Contains(disabled.Body.String(), "web_account_disabled") {
		t.Fatalf("disabled login response = %d %q", disabled.Code, disabled.Body.String())
	}
	saved.Disabled = false
	s.db.Save(&saved)
	wrong := invokeHandler(s.loginPost, formRequest(http.MethodPost, "/login", url.Values{"callsign": {"EA7KLK"}, "password": {"wrong"}}))
	if wrong.Code != http.StatusOK || !strings.Contains(wrong.Body.String(), "web_wrong_login") {
		t.Fatalf("wrong login response = %d %q", wrong.Code, wrong.Body.String())
	}
	sessionsBefore := len(s.sessions)
	login := invokeHandler(s.loginPost, formRequest(http.MethodPost, "/login", url.Values{"callsign": {"ea7klk"}, "password": {"secret"}}))
	assertRedirect(t, login, "/")
	if len(s.sessions) != sessionsBefore+1 {
		t.Fatalf("successful login did not create a session: before=%d after=%d", sessionsBefore, len(s.sessions))
	}

	profileForm := url.Values{
		"full_name":   {"Updated Operator"},
		"email":       {"updated@example.invalid"},
		"maidenhead":  {"im77ah"},
		"language":    {"fr"},
		"enable_aprs": {"false"},
		"qth":         {"Cadiz"},
		"rig":         {"HF"},
	}
	profile := invokeUserHandler(s.profilePost, formRequest(http.MethodPost, "/profile", profileForm), &saved)
	assertRedirect(t, profile, "/profile?msg=web_profile_saved")

	var updated dbUser
	if err := s.db.First(&updated, "callsign = ?", "EA7KLK").Error; err != nil {
		t.Fatal(err)
	}
	if updated.FullName != "Updated Operator" || updated.Language != "fr" || updated.QTH != "Cadiz" || updated.PasswordHash != saved.PasswordHash {
		t.Fatalf("profile update = %#v", updated)
	}
	invalidProfile := invokeUserHandler(s.profilePost, formRequest(http.MethodPost, "/profile", func() url.Values { v := profileForm; v.Set("email", "bad"); return v }()), &updated)
	if invalidProfile.Code != http.StatusOK || !strings.Contains(invalidProfile.Body.String(), "web_required_profile") {
		t.Fatalf("invalid profile response = %d %q", invalidProfile.Code, invalidProfile.Body.String())
	}
}

func TestAuthAuxiliaryHandlersAndSessions(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Operator", Email: "a@example.invalid", PasswordHash: hashPassword("secret")})
	if got := invokeHandler(s.loginForm, httptest.NewRequest(http.MethodGet, "/login?msg=hello", nil)); got.Code != http.StatusOK {
		t.Fatalf("loginForm status = %d", got.Code)
	}
	register := invokeHandler(s.registerForm, httptest.NewRequest(http.MethodGet, "/register?callsign=ea7klk", nil))
	if register.Code != http.StatusOK || !strings.Contains(register.Body.String(), "EA7KLK") {
		t.Fatalf("registerForm response = %d %q", register.Code, register.Body.String())
	}
	profile := invokeUserHandler(s.profileForm, httptest.NewRequest(http.MethodGet, "/profile?msg=saved", nil), user)
	if profile.Code != http.StatusOK {
		t.Fatalf("profileForm status = %d", profile.Code)
	}
	if directory := invokeUserHandler(s.directory, httptest.NewRequest(http.MethodGet, "/directory", nil), user); directory.Code != http.StatusOK {
		t.Fatalf("directory status = %d", directory.Code)
	}
	if home := invokeUserHandler(s.home, httptest.NewRequest(http.MethodGet, "/", nil), user); home.Code != http.StatusOK {
		t.Fatalf("home status = %d", home.Code)
	}

	invalidLanguage := invokeUserHandler(s.languagePost, formRequest(http.MethodPost, "/language", url.Values{"language": {"xx"}}), user)
	if invalidLanguage.Code != http.StatusBadRequest || !strings.Contains(invalidLanguage.Body.String(), "web_invalid_language") {
		t.Fatalf("invalid language response = %d %q", invalidLanguage.Code, invalidLanguage.Body.String())
	}
	unsafeReturn := invokeUserHandler(s.languagePost, formRequest(http.MethodPost, "/language", url.Values{"language": {"de"}, "return_to": {"//evil.example"}}), user)
	assertRedirect(t, unsafeReturn, "/")

	response := httptest.NewRecorder()
	s.setSession(response, user.Callsign)
	cookie := response.Result().Cookies()[0]
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(cookie)
	current, err := s.currentUser(request)
	if err != nil || current.Callsign != user.Callsign {
		t.Fatalf("currentUser() = %#v, %v", current, err)
	}
	noCookie, err := s.currentUser(httptest.NewRequest(http.MethodGet, "/", nil))
	if noCookie != nil || err != errUnauthorized {
		t.Fatalf("currentUser(no cookie) = %#v, %v", noCookie, err)
	}
	unknownCookie := httptest.NewRequest(http.MethodGet, "/", nil)
	unknownCookie.AddCookie(&http.Cookie{Name: "bbs_session", Value: "unknown"})
	if _, err := s.currentUser(unknownCookie); err != errUnauthorized {
		t.Fatalf("currentUser(unknown cookie) error = %v", err)
	}
	user.Disabled = true
	s.db.Save(user)
	if _, err := s.currentUser(request); err != errUnauthorized {
		t.Fatalf("currentUser(disabled) error = %v", err)
	}
	user.Disabled = false
	s.db.Save(user)
	s.cfg.sysops[user.Callsign] = true
	if _, err := s.currentUser(request); err != nil || !userFromDBIsSysop(t, s, user.Callsign) {
		t.Fatalf("configured sysop was not promoted: %v", err)
	}

	logout := invokeHandler(s.logoutPost, request)
	assertRedirect(t, logout, "/login")
	if _, err := s.currentUser(request); err != errUnauthorized {
		t.Fatal("cleared session remained valid")
	}
}

func userFromDBIsSysop(t *testing.T, s *server, callsign string) bool {
	t.Helper()
	var user dbUser
	if err := s.db.First(&user, "callsign = ?", callsign).Error; err != nil {
		t.Fatal(err)
	}
	return user.IsSysop
}
