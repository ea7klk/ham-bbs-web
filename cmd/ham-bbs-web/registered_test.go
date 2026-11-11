package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// A new account records its registration date, taken from the same stamp as
// first-seen so both columns agree with what the SSH BBS writes.
func TestRegistrationRecordsDateOnce(t *testing.T) {
	s := newTestServer(t)
	invokeHandler(s.registerPost, formRequest(http.MethodPost, "/register", validUserForm()))

	var stored dbUser
	if err := s.db.First(&stored, "callsign = ?", "EA7KLK").Error; err != nil {
		t.Fatal(err)
	}
	if stored.Registered == "" {
		t.Fatal("registration did not record a date")
	}
	if stored.Registered != stored.FirstSeen {
		t.Fatalf("registered = %q, first seen = %q; they must agree at creation", stored.Registered, stored.FirstSeen)
	}
	first := stored.Registered

	// A later profile change must not move it.
	form := validUserForm()
	form.Set("full_name", "Renamed Operator")
	form.Del("new_password")
	form.Del("verify_password")
	response := invokeUserHandler(s.profilePost, formRequest(http.MethodPost, "/profile", form), &stored)
	assertRedirect(t, response, "/profile?msg=web_profile_saved")

	var after dbUser
	if err := s.db.First(&after, "callsign = ?", "EA7KLK").Error; err != nil {
		t.Fatal(err)
	}
	if after.Registered != first {
		t.Fatalf("registration date moved on a profile change: %q -> %q", first, after.Registered)
	}
	if after.FirstSeen != first {
		t.Fatalf("first seen moved on a profile change: %q -> %q", first, after.FirstSeen)
	}
}

// The profile form builds a fresh account from the request, so a careless Save
// blanks any column it does not copy. The registration date is one of them.
func TestProfileSavePreservesRegistrationDate(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{
		Callsign:     "EA1ABC",
		FullName:     "Operator",
		Email:        "a@example.invalid",
		Language:     "en",
		PasswordHash: hashPassword("secret"),
		FirstSeen:    "2026-03-04 10:00 UTC",
		LastSeen:     "2026-03-04 10:00 UTC",
		Registered:   "2026-03-04 10:00 UTC",
	})

	form := url.Values{
		"full_name":       {"Renamed"},
		"email":           {"renamed@example.invalid"},
		"language":        {"es"},
		"qth":             {"Madrid"},
		"rig":             {"FT-817"},
		"new_password":    {"newsecret"},
		"verify_password": {"newsecret"},
	}
	assertRedirect(t, invokeUserHandler(s.profilePost, formRequest(http.MethodPost, "/profile", form), user), "/profile?msg=web_profile_saved")

	var after dbUser
	if err := s.db.First(&after, "callsign = ?", "EA1ABC").Error; err != nil {
		t.Fatal(err)
	}
	if after.FullName != "Renamed" {
		t.Fatalf("profile edit did not apply: %#v", after)
	}
	if after.Registered != "2026-03-04 10:00 UTC" {
		t.Fatalf("registration date was lost on profile save: %q", after.Registered)
	}
}

// Login writes the seen stamps, so it rewrites the row on every sign-in.
func TestLoginPreservesRegistrationDate(t *testing.T) {
	s := newTestServer(t)
	testUser(t, s, dbUser{
		Callsign:     "EA1ABC",
		FullName:     "Operator",
		Email:        "a@example.invalid",
		Language:     "en",
		PasswordHash: hashPassword("secret"),
		FirstSeen:    "2026-03-04 10:00 UTC",
		Registered:   "2026-03-04 10:00 UTC",
	})

	request := formRequest(http.MethodPost, "/login", url.Values{"callsign": {"EA1ABC"}, "password": {"secret"}})
	assertRedirect(t, invokeHandler(s.loginPost, request), "/")

	var after dbUser
	if err := s.db.First(&after, "callsign = ?", "EA1ABC").Error; err != nil {
		t.Fatal(err)
	}
	if after.Registered != "2026-03-04 10:00 UTC" {
		t.Fatalf("registration date was lost on login: %q", after.Registered)
	}
	if after.LastSeen == "" || after.LastSeen == "2026-03-04 10:00 UTC" {
		t.Fatalf("login did not update the last-seen stamp: %q", after.LastSeen)
	}
}

// The sysop form changes two columns; it must not rewrite the rest of the row.
func TestAdminUpdatePreservesRegistrationDate(t *testing.T) {
	s := newTestServer(t)
	actor := testUser(t, s, dbUser{Callsign: "EA1AAA", FullName: "Actor", Email: "a@example.invalid", IsSysop: true})
	testUser(t, s, dbUser{
		Callsign:   "EA1ABC",
		FullName:   "Target",
		Email:      "t@example.invalid",
		FirstSeen:  "2026-03-04 10:00 UTC",
		Registered: "2026-03-04 10:00 UTC",
	})

	form := url.Values{"is_sysop": {"true"}, "disabled": {"false"}}
	response := invokeUserHandler(s.adminUserUpdate, setPathValue(formRequest(http.MethodPost, "/admin/users/EA1ABC", form), "callsign", "EA1ABC"), actor)
	assertRedirect(t, response, "/admin/users")

	var after dbUser
	if err := s.db.First(&after, "callsign = ?", "EA1ABC").Error; err != nil {
		t.Fatal(err)
	}
	if !after.IsSysop {
		t.Fatal("the sysop change did not apply")
	}
	if after.Registered != "2026-03-04 10:00 UTC" {
		t.Fatalf("registration date was lost on a sysop update: %q", after.Registered)
	}
}

// Accounts created before the column existed take their date from the
// first-seen stamp, once, and only when there is a stamp to derive it from.
func TestBackfillRegisteredFromFirstSeen(t *testing.T) {
	s := newTestServer(t)
	rows := []dbUser{
		{Callsign: "EA7KLK", FirstSeen: "2026-07-10 12:00 UTC"},
		{Callsign: "DL1ABC", FirstSeen: "2026-07-11 12:00 UTC", Registered: "2026-07-11 12:00 UTC"},
		{Callsign: "N0CALL"},
	}
	for _, row := range rows {
		if err := s.db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}

	if err := backfillRegistered(s.db); err != nil {
		t.Fatal(err)
	}
	load := func(callsign string) string {
		t.Helper()
		var user dbUser
		if err := s.db.First(&user, "callsign = ?", callsign).Error; err != nil {
			t.Fatal(err)
		}
		return user.Registered
	}

	if got := load("EA7KLK"); got != "2026-07-10 12:00 UTC" {
		t.Fatalf("backfilled date = %q", got)
	}
	if got := load("DL1ABC"); got != "2026-07-11 12:00 UTC" {
		t.Fatalf("an existing date was overwritten: %q", got)
	}
	if got := load("N0CALL"); got != "" {
		t.Fatalf("fabricated a date for a row with no first_seen: %q", got)
	}

	// Idempotent: a second run changes nothing.
	if err := backfillRegistered(s.db); err != nil {
		t.Fatal(err)
	}
	if got := load("EA7KLK"); got != "2026-07-10 12:00 UTC" {
		t.Fatalf("second backfill changed the date: %q", got)
	}

	// The new date is not treated as a fresh account's, so it must not reopen
	// anything that keys on an empty column.
	var missing int64
	s.db.Model(&dbUser{}).Where("registered = '' AND callsign <> ?", "N0CALL").Count(&missing)
	if missing != 0 {
		t.Fatalf("%d row(s) still have no registration date", missing)
	}
}

// Mechanism check: opening the database creates the column, so a fresh install
// and an upgraded one end up with the same schema.
func TestOpenDatabaseAddsRegisteredColumn(t *testing.T) {
	s := newTestServer(t)
	if err := s.db.Exec("UPDATE db_users SET registered = registered WHERE 1=0").Error; err != nil {
		t.Fatalf("the db_users table has no registered column: %v", err)
	}
}

// The sysop user list is where the SSH BBS shows this date, so it is where the
// web app shows it too.
func TestAdminUsersShowsRegistrationDate(t *testing.T) {
	s := newTestServer(t)
	actor := testUser(t, s, dbUser{Callsign: "EA1AAA", FullName: "Actor", Email: "a@example.invalid", IsSysop: true})
	testUser(t, s, dbUser{
		Callsign:   "EA1ABC",
		FullName:   "Dated",
		Email:      "d@example.invalid",
		Registered: "2026-03-04 10:00 UTC",
	})
	testUser(t, s, dbUser{
		Callsign: "EA2XYZ",
		FullName: "Undated",
		Email:    "u@example.invalid",
	})

	response := invokeUserHandler(s.adminUsers, formRequest(http.MethodGet, "/admin/users", nil), actor)
	if response.Code != http.StatusOK {
		t.Fatalf("adminUsers status = %d", response.Code)
	}
	body := response.Body.String()

	if !strings.Contains(body, "date_registered") && !strings.Contains(body, "Registered") {
		t.Fatalf("the user list has no registration column: %q", body)
	}
	if !strings.Contains(body, "2026-03-04 10:00 UTC") {
		t.Fatalf("the registration date is not shown: %q", body)
	}
	// A row with no date renders a dash rather than an empty cell, so the
	// columns cannot shift.
	if !strings.Contains(body, "<td>-</td>") {
		t.Fatalf("a missing registration date did not render a dash: %q", body)
	}
}
