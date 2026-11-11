package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func (s *server) loginForm(w http.ResponseWriter, r *http.Request) {
	s.view(w, r, "login", nil, nil, r.URL.Query().Get("msg"), "")
}

func (s *server) loginPost(w http.ResponseWriter, r *http.Request) {
	callsign := normalizeCallsign(r.FormValue("callsign"))
	password := r.FormValue("password")
	var user dbUser
	if err := s.db.First(&user, "callsign = ?", callsign).Error; err != nil {
		http.Redirect(w, r, "/register?callsign="+callsign, http.StatusSeeOther)
		return
	}
	key := authThrottleKey(r, user.Callsign)
	if wait := s.throttle.lockedFor(key); wait > 0 {
		s.logBBSAction(user.Callsign, "web_login_lockout", "address=%q remaining=%s", clientAddress(r), wait.Round(time.Second))
		s.view(w, r, "login", nil, nil, "", s.t(user.Language, "web_login_locked"))
		return
	}
	if user.Disabled {
		s.view(w, r, "login", nil, nil, "", s.t(user.Language, "web_account_disabled"))
		return
	}
	if !verifyPassword(password, user.PasswordHash) {
		if locked, wait := s.throttle.recordFailure(key); locked {
			s.logBBSAction(user.Callsign, "web_login_lockout", "address=%q remaining=%s", clientAddress(r), wait.Round(time.Second))
			s.view(w, r, "login", nil, nil, "", s.t(user.Language, "web_login_locked"))
			return
		}
		s.view(w, r, "login", nil, nil, "", s.t(user.Language, "web_wrong_login"))
		return
	}
	s.throttle.clear(key)
	// Activation runs only after the password checks out, so an unactivated
	// account cannot be claimed by whoever guesses the callsign first. The
	// password is right, so no session is created yet: the caller is sent to the
	// code form instead.
	if !user.Activated {
		s.logBBSAction(user.Callsign, "web_login_needs_activation", "address=%q", clientAddress(r))
		http.Redirect(w, r, "/activate?callsign="+url.QueryEscape(user.Callsign), http.StatusSeeOther)
		return
	}
	user.LastSeen = now()
	s.db.Save(&user)
	s.logBBSAction(user.Callsign, "web_login", "sysop=%t", s.isSysop(&user))
	s.background.Add(1)
	go func() {
		defer s.background.Done()
		s.sendLoginAPRSBeacon(user.Callsign, user)
	}()
	s.setSession(w, user.Callsign)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *server) registerForm(w http.ResponseWriter, r *http.Request) {
	s.view(w, r, "register", nil, map[string]any{"Callsign": normalizeCallsign(r.URL.Query().Get("callsign")), "Languages": languageChoices(), "User": dbUser{}}, "", "")
}

func (s *server) registerPost(w http.ResponseWriter, r *http.Request) {
	callsign := normalizeCallsign(r.FormValue("callsign"))
	user, errText := s.userFromForm(r, dbUser{Callsign: callsign}, true)
	if errText != "" {
		s.view(w, r, "register", nil, map[string]any{"Callsign": callsign, "Languages": languageChoices(), "User": user}, "", errText)
		return
	}
	var count int64
	s.db.Model(&dbUser{}).Where("callsign = ?", callsign).Count(&count)
	if count > 0 {
		s.view(w, r, "register", nil, map[string]any{"Callsign": callsign, "Languages": languageChoices(), "User": user}, "", s.t(user.Language, "web_duplicate_callsign"))
		return
	}
	// A new account costs a PBKDF2 hash and a row, so it is metered per address
	// exactly as the SSH BBS meters it.
	address := clientAddress(r)
	allowed, limitErr := s.registrationAllowed(address, time.Now())
	if limitErr != nil {
		s.logBBSAction(callsign, "web_registration_throttle_error", "address=%q error=%q", address, limitErr.Error())
		s.view(w, r, "register", nil, map[string]any{"Callsign": callsign, "Languages": languageChoices(), "User": user}, "", s.t(user.Language, "registration_rate_limited"))
		return
	}
	if !allowed {
		s.logBBSAction(callsign, "web_registration_blocked", "address=%q limit=%d window=%s", address, registrationLimit, registrationWindow)
		s.view(w, r, "register", nil, map[string]any{"Callsign": callsign, "Languages": languageChoices(), "User": user}, "", s.t(user.Language, "registration_rate_limited"))
		return
	}
	user.FirstSeen = now()
	user.LastSeen = user.FirstSeen
	// The registration date is written once, here, and never updated again. It
	// shares the first-seen stamp so an account created by either application
	// carries the same value in both columns, exactly as the SSH BBS does.
	user.Registered = user.FirstSeen
	// BBS_SYSOPS is a bootstrap grant: it applies only while no enabled sysop
	// exists, so a configured callsign registered later gets a plain account.
	user.IsSysop = s.isBootstrapSysop(user.Callsign)
	// Without a mail path there is no way to deliver a code, so activation is
	// skipped rather than locking the caller out of a deployment with no SMTP.
	// That is the same decision the SSH BBS makes.
	needsActivation := s.mail.Enabled()
	user.Activated = !needsActivation
	if err := s.db.Create(&user).Error; err != nil {
		s.view(w, r, "register", nil, map[string]any{"Callsign": callsign, "Languages": languageChoices(), "User": user}, "", err.Error())
		return
	}
	if err := s.recordRegistration(address, time.Now()); err != nil {
		s.logBBSAction(callsign, "web_registration_throttle_error", "address=%q error=%q", address, err.Error())
	}
	s.logBBSAction(user.Callsign, "web_register", "activated=%t", user.Activated)
	if needsActivation {
		if err := s.sendActivationCode(user); err != nil {
			// The account exists, so the caller can request another code rather
			// than register again.
			s.view(w, r, "activate", nil, map[string]any{"Callsign": callsign}, "", s.t(user.Language, "activation_mail_failed"))
			return
		}
		http.Redirect(w, r, "/activate?callsign="+url.QueryEscape(callsign), http.StatusSeeOther)
		return
	}
	s.setSession(w, user.Callsign)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *server) logoutPost(w http.ResponseWriter, r *http.Request) {
	s.clearSession(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (s *server) languagePost(w http.ResponseWriter, r *http.Request, user *dbUser) {
	language := strings.TrimSpace(r.FormValue("language"))
	if _, ok := languages[language]; !ok {
		http.Error(w, s.t(user.Language, "web_invalid_language"), http.StatusBadRequest)
		return
	}
	if err := s.db.Model(&dbUser{}).Where("callsign = ?", user.Callsign).Update("language", language).Error; err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.logBBSAction(user.Callsign, "web_language_update", "language=%q", language)
	returnTo := strings.TrimSpace(r.FormValue("return_to"))
	if returnTo == "" || !strings.HasPrefix(returnTo, "/") || strings.HasPrefix(returnTo, "//") {
		returnTo = "/"
	}
	http.Redirect(w, r, returnTo, http.StatusSeeOther)
}

func (s *server) home(w http.ResponseWriter, r *http.Request, user *dbUser) {
	var bulletinCount, boardCount, userCount, receivedCount int64
	s.db.Model(&dbBulletin{}).Count(&bulletinCount)
	s.db.Model(&dbBoard{}).Count(&boardCount)
	s.db.Model(&dbUser{}).Count(&userCount)
	s.db.Model(&dbAPRSReceived{}).Where("user_callsign = ?", user.Callsign).Count(&receivedCount)

	// The dashboard leads with the newest bulletin, then lists the boards and the
	// bulletins before it: a count tells an operator nothing about where to go
	// next, and the announcement at the top of the board is the thing most likely
	// to have changed since their last visit.
	//
	// loadBulletins returns position ascending, and a new bulletin takes the next
	// position, so the newest is the *last* row — not the first.
	boards := s.loadBoards()
	bulletins := s.loadBulletins()
	var pinned *dbBulletin
	older := []dbBulletin{}
	if n := len(bulletins); n > 0 {
		pinned = &bulletins[n-1]
		for i := n - 2; i >= 0; i-- {
			older = append(older, bulletins[i])
		}
	}
	if len(older) > 3 {
		older = older[:3]
	}
	s.view(w, r, "home", user, map[string]any{
		"Bulletins":      bulletinCount,
		"Boards":         boardCount,
		"Users":          userCount,
		"Received":       receivedCount,
		"BoardList":      boards,
		"Counts":         s.boardMessageCounts(boards),
		"LatestBulletin": pinned,
		"Latest":         older,
	}, "", "")
}

func (s *server) profileForm(w http.ResponseWriter, r *http.Request, user *dbUser) {
	s.view(w, r, "profile", user, map[string]any{"Languages": languageChoices()}, r.URL.Query().Get("msg"), "")
}

func (s *server) profilePost(w http.ResponseWriter, r *http.Request, user *dbUser) {
	updated, errText := s.userFromForm(r, *user, false)
	if errText != "" {
		s.view(w, r, "profile", user, map[string]any{"Languages": languageChoices(), "Draft": updated}, "", errText)
		return
	}
	updated.Callsign = user.Callsign
	updated.FirstSeen = user.FirstSeen
	// Write-once fields: take them from the loaded row, never from the form,
	// which cannot carry them. userFromForm is already seeded with *user, so
	// this is belt and braces; it keeps the write-once set explicit beside the
	// fields that are deliberately allowed to change.
	updated.Registered = user.Registered
	updated.LastSeen = now()
	updated.IsSysop = user.IsSysop
	updated.Disabled = user.Disabled
	if r.FormValue("new_password") == "" {
		updated.PasswordHash = user.PasswordHash
	}

	// An address change needs two confirmations before it takes effect: one link
	// to the current address, one to the new one. The pending address is not
	// applied here, so a stolen session cannot redirect the account's mail.
	emailChanged := strings.TrimSpace(updated.Email) != strings.TrimSpace(user.Email)
	if emailChanged && s.mail.Enabled() {
		if err := s.beginEmailChange(user.Callsign, user.Email, updated.Email, updated.Language); err != nil {
			s.logBBSAction(user.Callsign, "web_email_change_failed", "from=%q to=%q error=%q", user.Email, updated.Email, err.Error())
			updated.Email = user.Email
			s.view(w, r, "profile", user, map[string]any{"Languages": languageChoices(), "Draft": updated}, "", err.Error())
			return
		}
		updated.Email = user.Email
		s.db.Save(&updated)
		s.logBBSAction(user.Callsign, "web_profile_update", "email_change=pending")
		http.Redirect(w, r, "/profile?msg="+url.QueryEscape(sprintfAll(s.t(updated.Language, "email_change_pending_detail"), user.Email)), http.StatusSeeOther)
		return
	}
	if emailChanged {
		// Without a mail path there is nothing to confirm with. Applying the
		// change keeps an unconfigured deployment usable, and the log records
		// that it was not verified.
		s.logBBSAction(user.Callsign, "web_email_change_unverified", "from=%q to=%q reason=%q", user.Email, updated.Email, "mail is not configured")
	}
	s.db.Save(&updated)
	s.logBBSAction(user.Callsign, "web_profile_update", "")
	http.Redirect(w, r, "/profile?msg="+url.QueryEscape(s.t(updated.Language, "web_profile_saved")), http.StatusSeeOther)
}

// sprintfAll substitutes the single %s in a translated message, which is how the
// shared translations carry a value (email_change_pending_detail names the
// address the confirmation went to). It falls back to the format itself rather
// than printing a Go formatting verb at a user.
func sprintfAll(format string, values ...any) string {
	if !strings.Contains(format, "%") {
		return format
	}
	return fmt.Sprintf(format, values...)
}

func (s *server) userFromForm(r *http.Request, user dbUser, requirePassword bool) (dbUser, string) {
	user.Callsign = normalizeCallsign(firstNonEmpty(user.Callsign, r.FormValue("callsign")))
	user.FullName = sanitizeSingleLineText(r.FormValue("full_name"))
	user.Email = sanitizeSingleLineText(r.FormValue("email"))
	user.Maidenhead = normalizeLocator(sanitizeSingleLineText(r.FormValue("maidenhead")))
	user.Language = strings.TrimSpace(r.FormValue("language"))
	user.EnableAPRS = r.FormValue("enable_aprs") == "true"
	user.QTH = sanitizeSingleLineText(r.FormValue("qth"))
	user.Rig = sanitizeSingleLineText(r.FormValue("rig"))
	lang := user.Language
	if lang == "" {
		lang = "en"
	}
	if !callsignRE.MatchString(user.Callsign) {
		return user, s.t(lang, "web_invalid_callsign")
	}
	if user.FullName == "" || !emailRE.MatchString(user.Email) {
		return user, s.t(lang, "web_required_profile")
	}
	if user.Language == "" {
		user.Language = "en"
	}
	if _, ok := languages[user.Language]; !ok {
		return user, s.t(lang, "web_invalid_language")
	}
	if user.Maidenhead != "" && !maidenheadRE.MatchString(user.Maidenhead) {
		return user, s.t(lang, "web_invalid_locator")
	}
	pass := r.FormValue("new_password")
	verify := r.FormValue("verify_password")
	if requirePassword || pass != "" || verify != "" {
		if pass == "" || pass != verify {
			return user, s.t(lang, "web_password_match")
		}
		// The SSH BBS rejects a short password before it hashes it; a password
		// that one surface accepts and the other refuses is a bug, not a style.
		if len(pass) < 8 {
			return user, s.t(lang, "password_short")
		}
		user.PasswordHash = hashPassword(pass)
	}
	return user, ""
}

func (s *server) directory(w http.ResponseWriter, r *http.Request, user *dbUser) {
	var users []dbUser
	s.db.Where("disabled = ?", false).Order("callsign").Find(&users)
	s.view(w, r, "directory", user, users, "", "")
}
