package main

import (
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Public account pages: activation, password reset, and email-change
// confirmation.
//
// These are the browser end of the flows whose state lives in the shared tables.
// The classic SSH BBS issues the same codes and tokens, so a link it mails lands
// here and an account it registered can be activated here; nothing is exchanged
// between the two services beyond the database.

// langFor returns the language to render a page in for a caller who may not be
// signed in. Without a session the deployment language is used, which is the
// only honest choice: the alternative is guessing from a header we do not read.
func (s *server) pageLang(user *dbUser) string {
	if user != nil && user.Language != "" {
		return user.Language
	}
	return "en"
}

// activateForm shows the activation form. It is reachable straight after
// registration and again from the login screen, so it names the callsign in the
// query string rather than requiring a session.
func (s *server) activateForm(w http.ResponseWriter, r *http.Request, user *dbUser) {
	callsign := normalizeCallsign(r.URL.Query().Get("callsign"))
	s.view(w, r, "activate", user, map[string]any{"Callsign": callsign}, r.URL.Query().Get("msg"), "")
}

// activatePost verifies the code, or sends a fresh one. The account is only
// marked active once the code matches, which is what proves the caller can read
// the address the account was registered with.
func (s *server) activatePost(w http.ResponseWriter, r *http.Request, user *dbUser) {
	callsign := normalizeCallsign(r.FormValue("callsign"))
	lang := s.pageLang(user)
	render := func(msg, errText string) {
		s.view(w, r, "activate", user, map[string]any{"Callsign": callsign}, msg, errText)
	}
	var profile dbUser
	if err := s.db.Where("callsign = ?", callsign).Limit(1).Find(&profile).Error; err != nil {
		render("", s.t(lang, "user_not_found"))
		return
	}
	if profile.Callsign == "" {
		render("", s.t(lang, "user_not_found"))
		return
	}
	if profile.Activated {
		render(s.t(lang, "web_activated_already"), "")
		return
	}

	if strings.TrimSpace(profile.Email) == "" {
		render("", s.t(lang, "activation_no_email"))
		return
	}
	if !s.mail.Enabled() {
		// No way to deliver a code. The SSH BBS marks the account active rather
		// than locking the caller out of a deployment with no SMTP; mirror that,
		// and say so in the log.
		s.logBBSAction(callsign, "activation_skipped", "reason=%q", "mail is not configured")
		if err := s.db.Model(&dbUser{}).Where("callsign = ?", callsign).Update("activated", true).Error; err != nil {
			render("", err.Error())
			return
		}
		render(s.t(lang, "web_activated_ok"), "")
		return
	}

	if r.FormValue("action") == "resend" {
		if err := s.sendActivationCode(profile); err != nil {
			render("", s.t(lang, "activation_mail_failed"))
			return
		}
		render(s.t(lang, "activation_resent"), "")
		return
	}

	code := normalizeActivationCode(r.FormValue("code"))
	if code == "" {
		render("", s.t(lang, "required"))
		return
	}
	err := s.verifyActivationCode(callsign, code, time.Now())
	switch {
	case err == nil:
		if err := s.db.Model(&dbUser{}).Where("callsign = ?", callsign).Update("activated", true).Error; err != nil {
			render("", err.Error())
			return
		}
		s.logBBSAction(callsign, "activation_confirmed", "")
		render(s.t(lang, "web_activated_ok"), "")
	case errors.Is(err, errActivationTooMany):
		s.logBBSAction(callsign, "activation_exhausted", "")
		render("", s.t(lang, "activation_too_many"))
	case errors.Is(err, errActivationExpired):
		if sendErr := s.sendActivationCode(profile); sendErr != nil {
			render("", s.t(lang, "activation_mail_failed"))
			return
		}
		render("", s.t(lang, "activation_expired"))
	default:
		render("", s.t(lang, "activation_wrong"))
	}
}

// sendActivationCode issues a code and mails it to the address on the account.
func (s *server) sendActivationCode(profile dbUser) error {
	code, err := s.issueActivationCode(profile.Callsign, time.Now())
	if err != nil {
		s.logBBSAction(profile.Callsign, "activation_code_failed", "error=%q", err.Error())
		return err
	}
	lang := profile.Language
	if lang == "" {
		lang = "en"
	}
	msg := activationMail(lang, profile.Callsign, code, s.cfg.name)
	msg.To = profile.Email
	if err := s.mail.Send(msg); err != nil {
		s.logBBSAction(profile.Callsign, "activation_mail_failed", "to=%q error=%q", profile.Email, err.Error())
		return err
	}
	s.logBBSAction(profile.Callsign, "activation_mail_sent", "to=%q", profile.Email)
	return nil
}

func (s *server) forgotForm(w http.ResponseWriter, r *http.Request, user *dbUser) {
	s.view(w, r, "forgot", user, nil, "", "")
}

// forgotPost answers identically whether or not the account exists. This is the
// one place where a difference would be useful to an attacker, and it costs
// nothing to keep quiet.
func (s *server) forgotPost(w http.ResponseWriter, r *http.Request, user *dbUser) {
	lang := s.pageLang(user)
	identifier := sanitizeSingleLineText(r.FormValue("identifier"))
	if identifier == "" {
		s.view(w, r, "forgot", user, nil, "", s.t(lang, "required"))
		return
	}
	s.logBBSAction("", "password_reset_requested", "identifier=%q", identifier)
	if !s.mail.Enabled() {
		s.view(w, r, "forgot", user, nil, "", s.t(lang, "mail_not_configured"))
		return
	}
	if err := s.sendPasswordReset(identifier); err != nil {
		s.logBBSAction("", "password_reset_failed", "identifier=%q error=%q", identifier, err.Error())
	}
	s.view(w, r, "forgot", user, nil, s.t(lang, "forgot_password_sent"), "")
}

// resetForm shows the new-password form for a live token. It peeks at the token
// rather than consuming it, so a reload or a mail-client prefetch does not spend
// the link.
func (s *server) resetForm(w http.ResponseWriter, r *http.Request, user *dbUser) {
	lang := s.pageLang(user)
	raw := r.URL.Query().Get("token")
	row, live := s.peekToken(raw)
	if !live || row.Purpose != tokenPurposePasswordReset {
		s.view(w, r, "reset", user, map[string]any{"Token": "", "Callsign": ""}, "", s.t(lang, "web_token_invalid"))
		return
	}
	s.view(w, r, "reset", user, map[string]any{"Token": raw, "Callsign": row.Callsign}, "", "")
}

// resetPost consumes the token and stores the new password.
func (s *server) resetPost(w http.ResponseWriter, r *http.Request, user *dbUser) {
	lang := s.pageLang(user)
	raw := r.FormValue("token")
	password := r.FormValue("new_password")
	verify := r.FormValue("verify_password")
	if len(password) < 8 {
		s.view(w, r, "reset", user, map[string]any{"Token": raw, "Callsign": ""}, "", s.t(lang, "password_short"))
		return
	}
	if password != verify {
		s.view(w, r, "reset", user, map[string]any{"Token": raw, "Callsign": ""}, "", s.t(lang, "password_mismatch"))
		return
	}
	row, err := s.claimToken(raw, tokenPurposePasswordReset, time.Now())
	if err != nil {
		s.view(w, r, "reset", user, map[string]any{"Token": "", "Callsign": ""}, "", s.t(lang, "web_token_invalid"))
		return
	}
	if err := s.db.Model(&dbUser{}).Where("callsign = ?", row.Callsign).
		Update("password_hash", hashPassword(password)).Error; err != nil {
		s.view(w, r, "reset", user, map[string]any{"Token": "", "Callsign": ""}, "", err.Error())
		return
	}
	s.logBBSAction(row.Callsign, "password_reset_completed", "")
	s.view(w, r, "login", nil, nil, s.t(lang, "web_password_reset"), "")
}

// emailConfirm consumes one half of an email change. The change is applied only
// once both halves have been consumed, so a stolen session cannot redirect mail
// by itself.
func (s *server) emailConfirm(w http.ResponseWriter, r *http.Request, user *dbUser) {
	lang := s.pageLang(user)
	raw := r.URL.Query().Get("token")
	peek, live := s.peekToken(raw)
	if !live || (peek.Purpose != tokenPurposeEmailOld && peek.Purpose != tokenPurposeEmailNew) {
		s.view(w, r, "email_confirm", user, map[string]any{"Done": false}, "", s.t(lang, "web_token_invalid"))
		return
	}
	row, err := s.claimToken(raw, peek.Purpose, time.Now())
	if err != nil {
		s.view(w, r, "email_confirm", user, map[string]any{"Done": false}, "", s.t(lang, "web_token_invalid"))
		return
	}
	s.logBBSAction(row.Callsign, "email_change_confirmed", "purpose=%q", row.Purpose)

	// Read the state of this change including the half that was consumed
	// earlier, then apply the address only when both halves are in.
	change, found, err := s.emailChangeState(row.ChangeID)
	if err != nil {
		s.view(w, r, "email_confirm", user, map[string]any{"Done": false}, "", err.Error())
		return
	}
	if !found {
		s.view(w, r, "email_confirm", user, map[string]any{"Done": false}, "", s.t(lang, "web_token_invalid"))
		return
	}
	if !change.OldDone || !change.NewDone {
		s.view(w, r, "email_confirm", user, map[string]any{"Done": true, "Applied": false, "Change": change}, s.t(lang, "web_email_change_half"), "")
		return
	}
	// Applying twice is harmless: a duplicate click on the second link finds the
	// address already set and reports the same success.
	if _, err := s.completeEmailChange(change); err != nil {
		s.view(w, r, "email_confirm", user, map[string]any{"Done": false}, "", err.Error())
		return
	}
	s.view(w, r, "email_confirm", user, map[string]any{"Done": true, "Applied": true, "Change": change}, s.t(lang, "web_email_change_applied"), "")
}

// boardShowMessageEdit renders the sysop message editor, which the SSH BBS has
// had since it allowed correcting a post without deleting the thread.
func (s *server) messageEditForm(w http.ResponseWriter, r *http.Request, user *dbUser) {
	msg, err := s.findMessage(r.PathValue("id"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	board, err := s.findBoard(msg.BoardID)
	if err != nil {
		s.notFound(w, r)
		return
	}
	s.view(w, r, "message_form", user, map[string]any{"Board": board, "Message": msg}, r.URL.Query().Get("msg"), "")
}

func (s *server) messageUpdate(w http.ResponseWriter, r *http.Request, user *dbUser) {
	msg, err := s.findMessage(r.PathValue("id"))
	if err != nil {
		s.notFound(w, r)
		return
	}
	board, err := s.findBoard(msg.BoardID)
	if err != nil {
		s.notFound(w, r)
		return
	}
	subject := sanitizeSingleLineText(r.FormValue("subject"))
	body := sanitizeMultilineText(r.FormValue("body"))
	if subject == "" || body == "" {
		s.view(w, r, "message_form", user, map[string]any{"Board": board, "Message": msg}, "", s.t(user.Language, "required"))
		return
	}
	if err := s.updateMessage(msg.ID, subject, body, now()); err != nil {
		s.view(w, r, "message_form", user, map[string]any{"Board": board, "Message": msg}, "", err.Error())
		return
	}
	s.logBBSAction(user.Callsign, "web_message_edit", "board=%q id=%d", board.ID, msg.ID)
	http.Redirect(w, r, "/boards/"+board.ID+"?msg="+url.QueryEscape(s.t(user.Language, "message_updated")), http.StatusSeeOther)
}

// about and resources are the two static screens the SSH BBS has in its main
// menu. They are read-only and identical for every signed-in caller.
func (s *server) about(w http.ResponseWriter, r *http.Request, user *dbUser) {
	s.view(w, r, "about", user, nil, "", "")
}

func (s *server) resources(w http.ResponseWriter, r *http.Request, user *dbUser) {
	s.view(w, r, "resources", user, nil, "", "")
}
