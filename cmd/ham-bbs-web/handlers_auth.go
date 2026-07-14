package main

import (
	"net/http"
	"net/url"
	"strings"
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
	if user.Disabled {
		s.view(w, r, "login", nil, nil, "", s.t(user.Language, "web_account_disabled"))
		return
	}
	if !verifyPassword(password, user.PasswordHash) {
		s.view(w, r, "login", nil, nil, "", s.t(user.Language, "web_wrong_login"))
		return
	}
	user.LastSeen = now()
	if s.cfg.sysops[user.Callsign] {
		user.IsSysop = true
	}
	s.db.Save(&user)
	s.logBBSAction(user.Callsign, "web_login", "sysop=%t", s.isSysop(&user))
	go s.sendLoginAPRSBeacon(user.Callsign, user)
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
	user.FirstSeen = now()
	user.LastSeen = now()
	user.IsSysop = s.cfg.sysops[user.Callsign]
	if err := s.db.Create(&user).Error; err != nil {
		s.view(w, r, "register", nil, map[string]any{"Callsign": callsign, "Languages": languageChoices(), "User": user}, "", err.Error())
		return
	}
	s.logBBSAction(user.Callsign, "web_register", "")
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
	s.view(w, r, "home", user, map[string]any{"Bulletins": bulletinCount, "Boards": boardCount, "Users": userCount, "Received": receivedCount}, "", "")
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
	updated.LastSeen = now()
	updated.IsSysop = user.IsSysop || s.cfg.sysops[user.Callsign]
	updated.Disabled = user.Disabled
	if r.FormValue("new_password") == "" {
		updated.PasswordHash = user.PasswordHash
	}
	s.db.Save(&updated)
	s.logBBSAction(user.Callsign, "web_profile_update", "")
	http.Redirect(w, r, "/profile?msg="+url.QueryEscape(s.t(updated.Language, "web_profile_saved")), http.StatusSeeOther)
}

func (s *server) userFromForm(r *http.Request, user dbUser, requirePassword bool) (dbUser, string) {
	user.Callsign = normalizeCallsign(firstNonEmpty(user.Callsign, r.FormValue("callsign")))
	user.FullName = strings.TrimSpace(r.FormValue("full_name"))
	user.Email = strings.TrimSpace(r.FormValue("email"))
	user.Maidenhead = normalizeLocator(r.FormValue("maidenhead"))
	user.Language = strings.TrimSpace(r.FormValue("language"))
	user.EnableAPRS = r.FormValue("enable_aprs") == "true"
	user.QTH = strings.TrimSpace(r.FormValue("qth"))
	user.Rig = strings.TrimSpace(r.FormValue("rig"))
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
		user.PasswordHash = hashPassword(pass)
	}
	return user, ""
}

func (s *server) directory(w http.ResponseWriter, r *http.Request, user *dbUser) {
	var users []dbUser
	s.db.Where("disabled = ?", false).Order("callsign").Find(&users)
	s.view(w, r, "directory", user, users, "", "")
}
