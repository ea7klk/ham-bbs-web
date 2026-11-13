package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// APRS bulletins, and the two bots.

// aprsphPage renders the bot command menu. The commands themselves are protocol
// keywords and are never translated; only the labels around them are.
func (s *server) aprsphPage(w http.ResponseWriter, r *http.Request, user *dbUser) {
	s.view(w, r, "aprs_aprsph", user, map[string]any{
		"Bot":   aprsBotAPRSPH,
		"Max":   aprsCommandMaxBody,
		"Lines": s.translationList(user.Language, "aprsph_help_lines"),
	}, "", "")
}

// aprsphSend handles one command from the menu.
//
// Every path needs APRS enabled on the account, for the same reason the send form
// does: without it the classic BBS will not transmit on this callsign's behalf,
// so offering a command here would be a promise the web app cannot keep.
func (s *server) aprsphSend(w http.ResponseWriter, r *http.Request, user *dbUser) {
	if !user.EnableAPRS {
		s.renderAPRSGate(w, r, user)
		return
	}

	command := strings.TrimSpace(r.FormValue("command"))
	kind := strings.TrimSpace(r.FormValue("kind"))
	count := strings.TrimSpace(r.FormValue("count"))
	target := strings.TrimSpace(r.FormValue("target"))
	phrase := strings.TrimSpace(r.FormValue("phrase"))
	thursday := r.FormValue("thursday") == "true"

	// The help text is local: it describes the command set and sends nothing.
	if kind == "help" || command == aprsphCmdHelp {
		s.logBBSAction(user.Callsign, "web_aprsph_help", "")
		data := s.aprsphViewData(user)
		data["Help"] = true
		s.view(w, r, "aprs_aprsph", user, data, "", "")
		return
	}

	// "raw" is the escape hatch: the caller typed the command themselves.
	if kind == "raw" {
		command = strings.TrimSpace(command)
	} else {
		// Validate before building, not after: a command assembled from an empty
		// phrase is not empty — "SEARCH10" is a valid-looking string that searches
		// for nothing, and the caller would never learn why.
		switch kind {
		case "search":
			if phrase == "" {
				s.view(w, r, "aprs_aprsph", user, s.aprsphViewData(user), "", s.t(user.Language, "required"))
				return
			}
		case "checkin":
			if phrase == "" {
				s.view(w, r, "aprs_aprsph", user, s.aprsphViewData(user), "", s.t(user.Language, "required"))
				return
			}
		case "mine", "inbox":
			if target != "" && !validAPRSCallsign(target) {
				s.view(w, r, "aprs_aprsph", user, s.aprsphViewData(user), "", s.t(user.Language, "web_invalid_callsign"))
				return
			}
		}
		command = aprsphCommand(kind, count, target, phrase, thursday)
	}
	if command == "" {
		s.view(w, r, "aprs_aprsph", user, s.aprsphViewData(user), "", s.t(user.Language, "required"))
		return
	}

	s.dispatchBotCommand(w, r, user, aprsBotAPRSPH, command, "web_aprsph_send")
}

// aprsphViewData is the data every render of the APRSPH page needs.
func (s *server) aprsphViewData(user *dbUser) map[string]any {
	return map[string]any{
		"Bot":   aprsBotAPRSPH,
		"Max":   aprsCommandMaxBody,
		"Lines": s.translationList(user.Language, "aprsph_help_lines"),
	}
}

// thursdayJoin checks in to #APRSThursday: "CQ HOTG" to the group server.
//
// Gated on the weekday, which is the group's own rule: it runs Thursdays
// 0000Z-2359Z, and traffic outside that is noise for everyone subscribed. The page
// says so rather than silently refusing.
func (s *server) thursdayJoin(w http.ResponseWriter, r *http.Request, user *dbUser) {
	if !user.EnableAPRS {
		s.renderAPRSGate(w, r, user)
		return
	}
	if !aprsThursdayActive(aprsClock()) {
		s.logBBSAction(user.Callsign, "web_aprs_thursday_blocked", "reason=%q", "not thursday")
		s.view(w, r, "aprs", user, s.aprsOverview(user), "", s.t(user.Language, "aprs_not_thursday_warning"))
		return
	}

	// A join carries a message, and the terminal client has always required one.
	// A bare "CQ HOTG" is accepted by APRS-IS and then ignored by the group
	// server: on 2026-11-12 the web form sent exactly that, the packet went out,
	// and no confirmation ever came back. The text is what the net relays.
	text := sanitizeSingleLineText(r.FormValue("text"))
	if text == "" {
		s.view(w, r, "aprs", user, s.aprsOverview(user), "", s.t(user.Language, "aprsph_checkin_text"))
		return
	}
	command := aprsHOTGJoin + " " + text
	if len([]rune(cleanAPRSBody(command))) > aprsCommandMaxBody {
		s.view(w, r, "aprs", user, s.aprsOverview(user), "",
			sprintfAll(s.t(user.Language, "aprs_command_too_long"), aprsCommandMaxBody, len([]rune(cleanAPRSBody(command)))))
		return
	}
	s.dispatchBotCommand(w, r, user, aprsBotANSRVR, command, "web_aprs_thursday_join")
}

// thursdayLeave unsubscribes: "U HOTG".
//
// Not gated on the weekday — #APRSThursday asks participants to leave when they
// stop, so their local RF network is not kept busy by the group's traffic, and
// that can be any day. It does require a confirmation, because it stops delivery
// of something they may have joined for.
func (s *server) thursdayLeave(w http.ResponseWriter, r *http.Request, user *dbUser) {
	if !user.EnableAPRS {
		s.renderAPRSGate(w, r, user)
		return
	}
	if r.FormValue("confirm") != "yes" {
		s.view(w, r, "aprs", user, s.aprsOverview(user), "", s.t(user.Language, "aprs_leave_confirm"))
		return
	}
	s.dispatchBotCommand(w, r, user, aprsBotANSRVR, aprsHOTGLeave, "web_aprs_thursday_leave")
}

// thursdayKeepalive extends the subscription: "K HOTG".
func (s *server) thursdayKeepalive(w http.ResponseWriter, r *http.Request, user *dbUser) {
	if !user.EnableAPRS {
		s.renderAPRSGate(w, r, user)
		return
	}
	s.dispatchBotCommand(w, r, user, aprsBotANSRVR, aprsHOTGKeep, "web_aprs_thursday_keepalive")
}

// dispatchBotCommand sends one command and renders the result on the APRS page.
//
// The result is shown rather than redirected on purpose: sending is an action
// that can fail for reasons the caller can act on — a refused length, a rejected
// packet — and a redirect would throw that away for a success message that might
// be wrong.
func (s *server) dispatchBotCommand(w http.ResponseWriter, r *http.Request, user *dbUser, destination, command, action string) {
	sent, err := s.sendAPRSCommand(user.Callsign, destination, command)
	if recordErr := s.addSentRecord(user.Callsign, sent); recordErr != nil {
		// The record is the history, not the transmission. Log it and carry on:
		// the caller still needs to know whether the packet went out.
		s.logBBSAction(user.Callsign, "web_aprs_history_failed", "error=%q", recordErr.Error())
	}
	s.logBBSAction(user.Callsign, action, "to=%q command=%q ok=%t", destination, command, err == nil)

	if err != nil {
		detail := err.Error()
		var tooLong errCommandTooLong
		if errors.As(err, &tooLong) {
			detail = sprintfAll(s.t(user.Language, "aprs_command_too_long"), tooLong.max, tooLong.got)
			if detail == "aprs_command_too_long" {
				detail = err.Error()
			}
		}
		s.view(w, r, "aprs", user, s.aprsOverview(user), "", detail)
		return
	}
	// Echo what went out: the command is protocol the caller typed, and seeing it
	// exactly as transmitted is the difference between "something happened" and
	// "the right thing happened".
	s.view(w, r, "aprs", user, s.aprsOverview(user),
		s.t(user.Language, "aprs_send_success")+" "+destination+": "+command, "")
}

// renderAPRSGate explains that APRS is off for the account, on the APRS page where
// the switch lives.
func (s *server) renderAPRSGate(w http.ResponseWriter, r *http.Request, user *dbUser) {
	s.view(w, r, "aprs", user, s.aprsOverview(user), "", s.t(user.Language, "aprs_enable_required"))
}

// translationList returns a translated multi-line value, for keys whose value is
// an array of strings rather than one string — currently the APRSPH help lines.
func (s *server) translationList(lang, key string) []string {
	raw, ok := s.text[lang][key]
	if !ok {
		raw = s.text["en"][key]
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, sprint(item))
	}
	return out
}

// sprint renders one translation array element as a string.
func sprint(value any) string {
	return fmt.Sprint(value)
}
