package main

import (
	"bufio"
	"fmt"
	"gorm.io/gorm"
	"math"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultAPRSPageSize = 10

var aprsPageSizes = []int{10, 25, 50}

func (s *server) aprs(w http.ResponseWriter, r *http.Request, user *dbUser) {
	s.view(w, r, "aprs", user, s.aprsOverview(user), "", "")
}

func (s *server) aprsSent(w http.ResponseWriter, r *http.Request, user *dbUser) {
	scope, ok := s.scopeFromRequest(w, r)
	if !ok {
		return
	}
	s.aprsSentScope(w, r, user, scope)
}

func (s *server) aprsReceived(w http.ResponseWriter, r *http.Request, user *dbUser) {
	scope, ok := s.scopeFromRequest(w, r)
	if !ok {
		return
	}
	s.aprsReceivedScope(w, r, user, scope)
}

// scopeFromRequest reads ?section= and reports an unknown value rather than
// silently falling back to the direct section, so a stale link says so.
func (s *server) scopeFromRequest(w http.ResponseWriter, r *http.Request) (aprsScope, bool) {
	scope, ok := aprsScopeOfParam(r.URL.Query().Get("section"))
	if !ok {
		s.notFound(w, r)
		return "", false
	}
	return scope, true
}

// aprsSentScope renders the sent history for one section.
func (s *server) aprsSentScope(w http.ResponseWriter, r *http.Request, user *dbUser, scope aprsScope) {
	path := aprsListPath("sent", scope)
	page, perPage := parseAPRSPage(r)
	messages, pagination := s.sentAPRSPage(user.Callsign, page, perPage, path, scope)
	tabs := s.aprsSectionTabs(user.Language, "sent", s.sentScopeCounts(user.Callsign), scope)
	s.view(w, r, "aprs_sent", user, aprsSentPageView{
		Lang:        user.Language,
		Rows:        messages,
		Pagination:  pagination,
		Section:     scope,
		SectionName: s.t(user.Language, scope.labelKey()),
		Sections:    tabs,
		ReceivedURL: aprsListPath("received", scope),
	}, "", "")
}

// aprsListPath is the URL for one direction and section. The direct section has
// no parameter, so those URLs stay the plain ones.
func aprsListPath(direction string, scope aprsScope) string {
	path := "/aprs/" + direction
	if value := scope.queryValue(); value != "" {
		path += "?section=" + url.QueryEscape(value)
	}
	return path
}

// aprsReceivedScope renders the received history for one section.
func (s *server) aprsReceivedScope(w http.ResponseWriter, r *http.Request, user *dbUser, scope aprsScope) {
	path := aprsListPath("received", scope)
	page, perPage := parseAPRSPage(r)
	messages, pagination := s.receivedAPRSPage(user.Callsign, page, perPage, path, scope)
	tabs := s.aprsSectionTabs(user.Language, "received", s.receivedScopeCounts(user.Callsign), scope)
	s.view(w, r, "aprs_received", user, aprsReceivedPageView{
		Lang:        user.Language,
		Rows:        messages,
		Pagination:  pagination,
		Section:     scope,
		SectionName: s.t(user.Language, scope.labelKey()),
		Sections:    tabs,
		SentURL:     aprsListPath("sent", scope),
	}, "", "")
}

func (s *server) aprsSendForm(w http.ResponseWriter, r *http.Request, user *dbUser) {
	// The classic BBS refuses to open the send form while APRS is off for the
	// profile, and it is the profile owner's own switch. Sending without it would
	// make the setting mean nothing here, which is exactly how it read when the
	// only control on the page was a label.
	if !user.EnableAPRS {
		s.view(w, r, "aprs", user, s.aprsOverview(user), "", s.t(user.Language, "aprs_enable_required"))
		return
	}
	s.view(w, r, "aprs_send", user, nil, "", "")
}

func parseAPRSPage(r *http.Request) (int, int) {
	page := 1
	if value, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && value > 0 {
		page = value
	}
	perPage := defaultAPRSPageSize
	if value, err := strconv.Atoi(r.URL.Query().Get("per_page")); err == nil {
		for _, allowed := range aprsPageSizes {
			if value == allowed {
				perPage = value
				break
			}
		}
	}
	return page, perPage
}

// loadSentHistory returns a user's whole sent history, newest first.
//
// The classic BBS bounds this (200 rows), so loading it is cheap and lets the
// section filter run in one place — see aprs_scope.go for why the filter is not
// expressed in SQL.
func (s *server) loadSentHistory(callsign string) []dbAPRSSent {
	var rows []dbAPRSSent
	s.db.Preload("Parts", func(db *gorm.DB) *gorm.DB { return db.Order("number") }).
		Where("user_callsign = ?", normalizeCallsign(callsign)).
		Order("position DESC, id DESC").
		Find(&rows)
	return rows
}

// loadReceivedHistory returns a user's whole received history, newest first.
func (s *server) loadReceivedHistory(callsign string) []dbAPRSReceived {
	var rows []dbAPRSReceived
	s.db.Where("user_callsign = ?", normalizeCallsign(callsign)).
		Order("position DESC, id DESC").
		Find(&rows)
	return rows
}

// sentAPRSPage returns one page of one section of the sent history.
//
// The filter is applied to the whole history before pagination, so a page is
// dense and the total counts only what the caller can actually see. Filtering a
// page after slicing would leave gaps and an inflated total.
func (s *server) sentAPRSPage(callsign string, page, perPage int, path string, scope aprsScope) ([]dbAPRSSent, aprsPagination) {
	all := s.loadSentHistory(callsign)
	filtered := make([]dbAPRSSent, 0, len(all))
	for _, row := range all {
		if aprsScopeOfMessage(row.To, row.Text) == scope {
			filtered = append(filtered, row)
		}
	}
	pagination := newAPRSPagination(path, page, perPage, int64(len(filtered)))
	return pageSlice(filtered, pagination), pagination
}

// receivedAPRSPage is the same for the received history.
func (s *server) receivedAPRSPage(callsign string, page, perPage int, path string, scope aprsScope) ([]dbAPRSReceived, aprsPagination) {
	all := s.loadReceivedHistory(callsign)
	filtered := make([]dbAPRSReceived, 0, len(all))
	for _, row := range all {
		if aprsScopeOfMessage(row.From, row.Text) == scope {
			filtered = append(filtered, row)
		}
	}
	pagination := newAPRSPagination(path, page, perPage, int64(len(filtered)))
	return pageSlice(filtered, pagination), pagination
}

// sentScopeCounts tallies each section of a user's sent history.
func (s *server) sentScopeCounts(callsign string) map[aprsScope]int64 {
	counts := map[aprsScope]int64{}
	for _, row := range s.loadSentHistory(callsign) {
		counts[aprsScopeOfMessage(row.To, row.Text)]++
	}
	return counts
}

// receivedScopeCounts tallies each section of a user's received history.
func (s *server) receivedScopeCounts(callsign string) map[aprsScope]int64 {
	counts := map[aprsScope]int64{}
	for _, row := range s.loadReceivedHistory(callsign) {
		counts[aprsScopeOfMessage(row.From, row.Text)]++
	}
	return counts
}

// pageSlice returns the rows for the pagination's current page.
func pageSlice[T any](rows []T, pagination aprsPagination) []T {
	start := (pagination.Page - 1) * pagination.PerPage
	if start >= len(rows) {
		return nil
	}
	end := start + pagination.PerPage
	if end > len(rows) {
		end = len(rows)
	}
	return rows[start:end]
}

func newAPRSPagination(path string, page, perPage int, total int64) aprsPagination {
	if perPage < 1 {
		perPage = defaultAPRSPageSize
	}
	pages := int((total + int64(perPage) - 1) / int64(perPage))
	if pages < 1 {
		pages = 1
	}
	if page < 1 {
		page = 1
	}
	if page > pages {
		page = pages
	}
	pagination := aprsPagination{
		Page:        page,
		PerPage:     perPage,
		Total:       total,
		Pages:       pages,
		PageSizes:   aprsPageSizes,
		HasPrevious: page > 1,
		HasNext:     page < pages,
	}
	if pagination.HasPrevious {
		pagination.PreviousURL = aprsPageURL(path, page-1, perPage)
	}
	if pagination.HasNext {
		pagination.NextURL = aprsPageURL(path, page+1, perPage)
	}
	return pagination
}

// aprsPageURL builds a pagination link. The separator depends on whether the
// base path already carries the section parameter.
func aprsPageURL(path string, page, perPage int) string {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%spage=%d&per_page=%d", path, sep, page, perPage)
}

func (s *server) aprsSentDetail(w http.ResponseWriter, r *http.Request, user *dbUser) {
	msg, err := s.findSentAPRS(r.PathValue("id"), user.Callsign)
	if err != nil {
		s.notFound(w, r)
		return
	}
	s.view(w, r, "aprs_sent_detail", user, map[string]any{"Message": msg}, "", "")
}

func (s *server) aprsReceivedDetail(w http.ResponseWriter, r *http.Request, user *dbUser) {
	msg, err := s.findReceivedAPRS(r.PathValue("id"), user.Callsign)
	if err != nil {
		s.notFound(w, r)
		return
	}
	detail := receivedAPRSDetail{
		Text: singleLineAPRSDetail(stripAPRSMessageID(msg.Text)),
		Raw:  singleLineAPRSDetail(msg.Raw),
	}
	s.view(w, r, "aprs_received_detail", user, map[string]any{"Message": msg, "Detail": detail}, "", "")
}

func (s *server) aprsReceivedReplyForm(w http.ResponseWriter, r *http.Request, user *dbUser) {
	msg, err := s.findReceivedAPRS(r.PathValue("id"), user.Callsign)
	if err != nil {
		s.notFound(w, r)
		return
	}
	destination, ssid := splitAPRSDestination(msg.From)
	detail := receivedAPRSDetail{
		Text: singleLineAPRSDetail(stripAPRSMessageID(msg.Text)),
		Raw:  singleLineAPRSDetail(msg.Raw),
	}
	s.view(w, r, "aprs_reply", user, map[string]any{
		"Message":         msg,
		"Detail":          detail,
		"Destination":     destination,
		"DestinationSSID": ssid,
	}, "", "")
}

func (s *server) aprsReceivedReply(w http.ResponseWriter, r *http.Request, user *dbUser) {
	msg, err := s.findReceivedAPRS(r.PathValue("id"), user.Callsign)
	if err != nil {
		s.notFound(w, r)
		return
	}
	// Replying transmits, so it needs the same gate as sending; otherwise the
	// reply form is a way around it. The message is resolved first so a stale
	// link is still a 404 rather than a policy message.
	if !user.EnableAPRS {
		s.logBBSAction(user.Callsign, "web_aprs_reply_blocked", "reason=%q", "aprs disabled for profile")
		s.view(w, r, "aprs", user, s.aprsOverview(user), "", s.t(user.Language, "aprs_enable_required"))
		return
	}
	destination := composeAPRSDestination(r.FormValue("destination"), r.FormValue("destination_ssid"))
	text := r.FormValue("text")
	sent, ok := s.sendAPRSMessage(user.Callsign, destination, text)
	_ = s.addSentRecord(user.Callsign, sent)
	s.logBBSAction(user.Callsign, "web_aprs_reply", "received_id=%d to=%q parts=%d ok=%t", msg.ID, destination, len(sent.Parts), ok)
	http.Redirect(w, r, "/aprs/received", http.StatusSeeOther)
}

func (s *server) aprsSentDelete(w http.ResponseWriter, r *http.Request, user *dbUser) {
	msg, err := s.findSentAPRS(r.PathValue("id"), user.Callsign)
	if err != nil {
		s.notFound(w, r)
		return
	}
	if _, err := s.deleteSentAPRS(user.Callsign, []uint{msg.ID}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.logBBSAction(user.Callsign, "web_aprs_sent_delete", "to=%q at=%q", msg.To, msg.At)
	http.Redirect(w, r, "/aprs/sent", http.StatusSeeOther)
}

func (s *server) aprsSentBulkDelete(w http.ResponseWriter, r *http.Request, user *dbUser) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ids := parseAPRSIDs(r.Form["sent_ids"])
	deleted, err := s.deleteSentAPRS(user.Callsign, ids)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.logBBSAction(user.Callsign, "web_aprs_sent_bulk_delete", "count=%d", deleted)
	http.Redirect(w, r, "/aprs/sent", http.StatusSeeOther)
}

func (s *server) aprsReceivedDelete(w http.ResponseWriter, r *http.Request, user *dbUser) {
	msg, err := s.findReceivedAPRS(r.PathValue("id"), user.Callsign)
	if err != nil {
		s.notFound(w, r)
		return
	}
	if _, err := s.deleteReceivedAPRS(user.Callsign, []uint{msg.ID}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.logBBSAction(user.Callsign, "web_aprs_received_delete", "from=%q at=%q", msg.From, msg.At)
	http.Redirect(w, r, "/aprs/received", http.StatusSeeOther)
}

func (s *server) aprsReceivedBulkDelete(w http.ResponseWriter, r *http.Request, user *dbUser) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ids := parseAPRSIDs(r.Form["received_ids"])
	deleted, err := s.deleteReceivedAPRS(user.Callsign, ids)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.logBBSAction(user.Callsign, "web_aprs_received_bulk_delete", "count=%d", deleted)
	http.Redirect(w, r, "/aprs/received", http.StatusSeeOther)
}

func (s *server) deleteSentAPRS(callsign string, ids []uint) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var deleted int64
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var rows []dbAPRSSent
		if err := tx.Where("user_callsign = ? AND id IN ?", callsign, ids).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		ownedIDs := make([]uint, 0, len(rows))
		for _, row := range rows {
			ownedIDs = append(ownedIDs, row.ID)
		}
		if err := tx.Delete(&dbAPRSSentPart{}, "sent_id IN ?", ownedIDs).Error; err != nil {
			return err
		}
		result := tx.Where("user_callsign = ? AND id IN ?", callsign, ownedIDs).Delete(&dbAPRSSent{})
		deleted = result.RowsAffected
		return result.Error
	})
	return int(deleted), err
}

func (s *server) deleteReceivedAPRS(callsign string, ids []uint) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	result := s.db.Where("user_callsign = ? AND id IN ?", callsign, ids).Delete(&dbAPRSReceived{})
	return int(result.RowsAffected), result.Error
}

func parseAPRSIDs(values []string) []uint {
	seen := map[uint]bool{}
	ids := make([]uint, 0, len(values))
	for _, raw := range values {
		id, err := strconv.ParseUint(raw, 10, 64)
		if err != nil || id == 0 || id > uint64(^uint(0)) {
			continue
		}
		value := uint(id)
		if seen[value] {
			continue
		}
		seen[value] = true
		ids = append(ids, value)
	}
	return ids
}

func (s *server) findSentAPRS(rawID, callsign string) (dbAPRSSent, error) {
	id, err := strconv.ParseUint(rawID, 10, 64)
	if err != nil || id == 0 {
		return dbAPRSSent{}, gorm.ErrRecordNotFound
	}
	var msg dbAPRSSent
	err = s.db.Preload("Parts", func(db *gorm.DB) *gorm.DB { return db.Order("number") }).
		Where("id = ? AND user_callsign = ?", id, callsign).
		First(&msg).Error
	return msg, err
}

func (s *server) findReceivedAPRS(rawID, callsign string) (dbAPRSReceived, error) {
	id, err := strconv.ParseUint(rawID, 10, 64)
	if err != nil || id == 0 {
		return dbAPRSReceived{}, gorm.ErrRecordNotFound
	}
	var msg dbAPRSReceived
	err = s.db.Where("id = ? AND user_callsign = ?", id, callsign).First(&msg).Error
	return msg, err
}

// aprsToggle switches this account's APRS flag. It is a single-click action: the
// button in the "aprs_toggle" template carries the value to write, so there is no
// second Save step to forget.
//
// The flag is not cosmetic. With it off the classic BBS will not deliver APRS
// messages addressed to this callsign, will not beacon on login, and — see
// aprsSendForm — this app will not transmit on the account's behalf.
func (s *server) aprsToggle(w http.ResponseWriter, r *http.Request, user *dbUser) {
	enabled := r.FormValue("enable_aprs") == "true"
	// Named columns only: a whole-row save would rewrite the write-once fields
	// from a copy read at the start of the request.
	if err := s.db.Model(&dbUser{}).Where("callsign = ?", user.Callsign).
		Updates(map[string]any{"enable_aprs": enabled, "last_seen": now()}).Error; err != nil {
		s.logBBSAction(user.Callsign, "web_aprs_toggle_failed", "enabled=%t error=%q", enabled, err.Error())
		s.view(w, r, "aprs", user, s.aprsOverview(user), "", err.Error())
		return
	}
	s.logBBSAction(user.Callsign, "web_aprs_toggle", "enabled=%t", enabled)
	http.Redirect(w, r, aprsReturnTarget(r.FormValue("return_to")), http.StatusSeeOther)
}

// aprsReturnTarget keeps the caller where they were. Only paths under /aprs are
// accepted, so the field cannot be used to bounce a signed-in caller to another
// site or to an arbitrary endpoint.
func aprsReturnTarget(raw string) string {
	target := strings.TrimSpace(raw)
	if target == "/aprs" || strings.HasPrefix(target, "/aprs/") {
		return target
	}
	return "/aprs"
}

// aprsOverview gathers the counts the APRS landing page shows.
func (s *server) aprsOverview(user *dbUser) aprsOverviewView {
	_, sentPagination := s.sentAPRSPage(user.Callsign, 1, defaultAPRSPageSize, "/aprs/sent", aprsScopeDirect)
	_, receivedPagination := s.receivedAPRSPage(user.Callsign, 1, defaultAPRSPageSize, "/aprs/received", aprsScopeDirect)
	return aprsOverviewView{
		Sent:           sentPagination,
		Received:       receivedPagination,
		ThursdayGroup:  aprsGroupHOTG,
		ThursdayActive: aprsThursdayActive(aprsClock()),
	}
}

func (s *server) aprsSend(w http.ResponseWriter, r *http.Request, user *dbUser) {
	// The form is not the only way in: a POST straight at this route would
	// otherwise transmit for an account that has APRS switched off.
	if !user.EnableAPRS {
		s.logBBSAction(user.Callsign, "web_aprs_send_blocked", "reason=%q", "aprs disabled for profile")
		s.view(w, r, "aprs", user, s.aprsOverview(user), "", s.t(user.Language, "aprs_enable_required"))
		return
	}
	destination := composeAPRSDestination(r.FormValue("destination"), r.FormValue("destination_ssid"))
	text := r.FormValue("text")
	sent, ok := s.sendAPRSMessage(user.Callsign, destination, text)
	_ = s.addSentRecord(user.Callsign, sent)
	s.logBBSAction(user.Callsign, "web_aprs_send", "to=%q parts=%d ok=%t", destination, len(sent.Parts), ok)
	http.Redirect(w, r, "/aprs/send", http.StatusSeeOther)
}

func (s *server) addSentRecord(callsign string, sent sentAPRS) error {
	callsign = normalizeCallsign(callsign)
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var maxPos int
		_ = tx.Model(&dbAPRSSent{}).Where("user_callsign = ?", callsign).Select("COALESCE(MAX(position), -1)").Scan(&maxPos).Error
		row := dbAPRSSent{UserCallsign: callsign, Position: maxPos + 1, At: sent.At, From: sent.From, To: sent.To, Text: singleLine(sent.Text), Status: normalizeAPRSStatus(sent.Status), Acked: sent.Acked, Passcode: sent.Passcode}
		if row.At == "" {
			row.At = now()
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		for i, part := range sent.Parts {
			number := part.Number
			if number == 0 {
				number = i + 1
			}
			if err := tx.Create(&dbAPRSSentPart{SentID: row.ID, Number: number, Text: singleLine(part.Text), Status: normalizeAPRSStatus(part.Status), Detail: singleLine(part.Detail), MessageID: part.MessageID, Acked: part.Acked}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return s.trimSentRows(callsign)
}

func (s *server) trimSentRows(callsign string) error {
	var rows []dbAPRSSent
	if err := s.db.Where("user_callsign = ?", normalizeCallsign(callsign)).Order("position DESC, id DESC").Find(&rows).Error; err != nil {
		return err
	}
	for i, row := range rows {
		if i >= sentHistoryLimit {
			if err := s.db.Delete(&dbAPRSSentPart{}, "sent_id = ?", row.ID).Error; err != nil {
				return err
			}
			if err := s.db.Delete(&dbAPRSSent{}, row.ID).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *server) sendAPRSMessage(source, destination, text string) (sentAPRS, bool) {
	if !validAPRSCallsign(source) {
		return sentAPRS{At: now(), Status: "failed", Parts: []sentAPRSPart{{Number: 1, Status: "failed", Detail: "invalid source callsign"}}}, false
	}
	if !validAPRSCallsign(destination) {
		return sentAPRS{At: now(), Status: "failed", Parts: []sentAPRSPart{{Number: 1, Status: "failed", Detail: "invalid destination callsign"}}}, false
	}
	from := aprsSSID0(source)
	to := normalizeAPRSCallsign(destination)
	passcode := aprsPasscode(from)
	parts := splitAPRSMessage(text)
	sent := sentAPRS{At: now(), From: from, To: to, Text: cleanAPRSBody(text), Status: "sent", Passcode: passcode}
	ids := aprsMessageIDs(from, to, len(parts), time.Now())
	sent.Parts, _ = s.sendAPRSParts(from, passcode, to, parts, ids)
	allOK := true
	for _, part := range sent.Parts {
		if normalizeAPRSStatus(part.Status) != "sent" {
			allOK = false
		}
	}
	if len(sent.Parts) == 0 || !allOK {
		sent.Status = "failed"
	}
	return sent, allOK
}

func (s *server) sendAPRSParts(source string, passcode int, destination string, parts []string, messageIDs []string) ([]sentAPRSPart, bool) {
	address := net.JoinHostPort(s.cfg.aprsServer, strconv.Itoa(s.cfg.aprsPort))
	conn, err := net.DialTimeout("tcp", address, 10*time.Second)
	if err != nil {
		detail := fmt.Sprintf("APRS-IS unreachable at %s: %v", address, err)
		s.logAPRSSendResult(source, destination, "", "", detail, err)
		return []sentAPRSPart{{Number: 1, Status: "failed", Detail: detail}}, false
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30*time.Second + time.Duration(len(parts))*3*time.Second))
	reader := bufio.NewReader(conn)
	loginLine := fmt.Sprintf("user %s pass %d vers HamNetBBSWeb 0.1\r\n", source, passcode)
	if _, err := conn.Write([]byte(loginLine)); err != nil {
		return []sentAPRSPart{{Number: 1, Status: "failed", Detail: err.Error()}}, false
	}
	response, err := readAPRSISLoginResponse(reader)
	if err != nil {
		s.logAPRSSendResult(source, destination, "", "", response, err)
		return []sentAPRSPart{{Number: 1, Status: "failed", Detail: err.Error()}}, false
	}
	out := make([]sentAPRSPart, 0, len(parts))
	allOK := true
	for i, part := range parts {
		msgID := ""
		if i < len(messageIDs) {
			msgID = messageIDs[i]
		}
		packetText := withAPRSMessageID(part, msgID)
		packet := formatAPRSMessagePacket(source, destination, packetText)
		status := "sent"
		detail := strings.TrimSpace(response)
		err := writeAPRSISPacket(conn, packet)
		if err != nil {
			status = "failed"
			detail = err.Error()
			allOK = false
		}
		s.logAPRSSendResult(source, destination, packetText, packet, response, err)
		out = append(out, sentAPRSPart{Number: i + 1, Text: part, Status: status, Detail: detail, MessageID: msgID})
		if !allOK {
			break
		}
		if i < len(parts)-1 {
			time.Sleep(750 * time.Millisecond)
		}
	}
	return out, allOK
}

func readAPRSISLoginResponse(reader *bufio.Reader) (string, error) {
	lines := []string{}
	for i := 0; i < 8; i++ {
		line, err := reader.ReadString('\n')
		if err != nil {
			return strings.Join(lines, ""), err
		}
		lines = append(lines, line)
		lower := strings.ToLower(line)
		if strings.Contains(lower, "logresp") {
			response := strings.Join(lines, "")
			if strings.Contains(lower, "unverified") || strings.Contains(lower, "invalid") {
				return response, fmt.Errorf("APRS-IS login failed: %s", strings.TrimSpace(line))
			}
			return response, nil
		}
	}
	return strings.Join(lines, ""), fmt.Errorf("APRS-IS login response did not include logresp")
}

func writeAPRSISPacket(conn net.Conn, packet string) error {
	_, err := fmt.Fprintf(conn, "%s\r\n", packet)
	return err
}

func (s *server) sendLoginAPRSBeacon(callsign string, profile dbUser) {
	if !profile.EnableAPRS || strings.TrimSpace(profile.Maidenhead) == "" {
		return
	}
	source := aprsSSID0(callsign)
	if !validAPRSCallsign(source) {
		s.logBBSAction(callsign, "aprs_beacon_skipped", "reason=%q source=%q", "invalid APRS callsign", source)
		return
	}
	lat, lon, err := maidenheadCenter(profile.Maidenhead)
	if err != nil {
		s.logBBSAction(callsign, "aprs_beacon_skipped", "locator=%q error=%q", profile.Maidenhead, err.Error())
		return
	}
	if err := s.sendAPRSBeacon(source, lat, lon, aprsBeaconText); err != nil {
		s.logBBSAction(callsign, "aprs_beacon_failed", "locator=%q lat=%.5f lon=%.5f error=%q", normalizeLocator(profile.Maidenhead), lat, lon, err.Error())
		return
	}
	s.logBBSAction(callsign, "aprs_beacon_sent", "locator=%q lat=%.5f lon=%.5f", normalizeLocator(profile.Maidenhead), lat, lon)
}

func (s *server) sendAPRSBeacon(source string, lat, lon float64, comment string) error {
	source = aprsSSID0(source)
	if !validAPRSCallsign(source) {
		return fmt.Errorf("invalid APRS beacon source: %s", source)
	}
	passcode := aprsPasscode(source)
	address := net.JoinHostPort(s.cfg.aprsServer, strconv.Itoa(s.cfg.aprsPort))
	conn, err := net.DialTimeout("tcp", address, 10*time.Second)
	if err != nil {
		detail := fmt.Sprintf("APRS-IS unreachable at %s: %v", address, err)
		s.logAPRSSendResult(source, "BEACON", comment, "", detail, err)
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))

	reader := bufio.NewReader(conn)
	loginLine := fmt.Sprintf("user %s pass %d vers HamNetBBSWeb 0.1\r\n", source, passcode)
	if _, err := conn.Write([]byte(loginLine)); err != nil {
		s.logAPRSSendResult(source, "BEACON", comment, "", "", err)
		return err
	}
	response, err := readAPRSISLoginResponse(reader)
	if err != nil {
		s.logAPRSSendResult(source, "BEACON", comment, "", response, err)
		return err
	}

	packet := formatAPRSBeaconPacket(source, lat, lon, comment)
	err = writeAPRSISPacket(conn, packet)
	s.logAPRSSendResult(source, "BEACON", comment, packet, response, err)
	return err
}

func formatAPRSMessagePacket(source, destination, text string) string {
	return fmt.Sprintf("%s>APRS,TCPIP*::%-9s:%s", aprsSSID0(source), normalizeAPRSCallsign(destination), text)
}

func formatAPRSBeaconPacket(source string, lat, lon float64, comment string) string {
	return fmt.Sprintf("%s>APRS,TCPIP*:%s", aprsSSID0(source), formatAPRSPosition(lat, lon, '\\', 'm', comment))
}

func formatAPRSPosition(lat, lon float64, symbolTable, symbolCode rune, comment string) string {
	latHemisphere := "N"
	if lat < 0 {
		latHemisphere = "S"
	}
	lonHemisphere := "E"
	if lon < 0 {
		lonHemisphere = "W"
	}
	lat = math.Abs(lat)
	lon = math.Abs(lon)
	latDegrees := int(lat)
	lonDegrees := int(lon)
	latMinutes := (lat - float64(latDegrees)) * 60
	lonMinutes := (lon - float64(lonDegrees)) * 60
	return fmt.Sprintf("!%02d%05.2f%s%c%03d%05.2f%s%c%s", latDegrees, latMinutes, latHemisphere, symbolTable, lonDegrees, lonMinutes, lonHemisphere, symbolCode, cleanAPRSBody(comment))
}

func withAPRSMessageID(text, messageID string) string {
	messageID = normalizeAPRSMessageID(messageID)
	if messageID == "" {
		return text
	}
	return text + "{" + messageID
}

func aprsMessageIDs(source, destination string, count int, at time.Time) []string {
	if count < 1 {
		return nil
	}
	seed := at.UnixMilli()
	for _, r := range normalizeAPRSCallsign(source) + normalizeAPRSCallsign(destination) {
		seed += int64(r)
	}
	const space = int64(36 * 36 * 36 * 36 * 36)
	out := make([]string, 0, count)
	for i := 0; i < count; i++ {
		value := (seed + int64(i)) % space
		id := strings.ToUpper(strconv.FormatInt(value, 36))
		if len(id) > 5 {
			id = id[len(id)-5:]
		}
		for len(id) < 5 {
			id = "0" + id
		}
		out = append(out, id)
	}
	return out
}

func aprsPasscode(callsign string) int {
	base := aprsBaseCallsign(callsign)
	code := 0x73e2
	for i, r := range base {
		if i%2 == 0 {
			code ^= int(r) << 8
		} else {
			code ^= int(r)
		}
	}
	return code & 0x7fff
}

func aprsBaseCallsign(callsign string) string {
	return strings.SplitN(normalizeAPRSCallsign(callsign), "-", 2)[0]
}

func aprsSSID0(callsign string) string {
	base := aprsBaseCallsign(callsign)
	if base == "" {
		return ""
	}
	return base + "-0"
}

func cleanAPRSBody(text string) string {
	body := strings.NewReplacer("\r", " ", "\n", " ").Replace(text)
	body = strings.Join(strings.Fields(body), " ")
	return asciiSafe(strings.ToValidUTF8(body, "?"))
}

func splitAPRSMessage(text string) []string {
	body := cleanAPRSBody(text)
	if body == "" {
		return []string{""}
	}
	if len([]rune(body)) <= aprsMessageLimit {
		return []string{body}
	}
	total := len(splitRunes(body, aprsMessageLimit))
	for {
		prefixWidth := len([]rune(fmt.Sprintf("[%d/%d] ", total, total)))
		chunkLimit := aprsMessageLimit - prefixWidth
		if chunkLimit < 1 {
			chunkLimit = 1
		}
		chunks := splitRunes(body, chunkLimit)
		if len(chunks) == total {
			out := make([]string, 0, total)
			for i, chunk := range chunks {
				out = append(out, fmt.Sprintf("[%d/%d] %s", i+1, total, chunk))
			}
			return out
		}
		total = len(chunks)
	}
}

func splitRunes(text string, limit int) []string {
	runes := []rune(text)
	out := []string{}
	for len(runes) > 0 {
		n := min(limit, len(runes))
		out = append(out, string(runes[:n]))
		runes = runes[n:]
	}
	return out
}

func singleLineAPRSDetail(text string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(text, "\n", " ")), " ")
}

func formatAPRSDateTime(value string) aprsTimestamp {
	parts := strings.Fields(strings.TrimSpace(value))
	if len(parts) == 0 {
		return aprsTimestamp{}
	}
	result := aprsTimestamp{Date: parts[0]}
	if len(parts) > 1 {
		result.Time = strings.Join(parts[1:], " ")
	}
	return result
}

func aprsListText(value string) string {
	return singleLineAPRSDetail(stripAPRSMessageID(value))
}

func stripAPRSMessageID(text string) string {
	body, _ := splitAPRSMessageID(text)
	return body
}

func splitAPRSMessageID(text string) (string, string) {
	text = strings.TrimSpace(text)
	idx := strings.LastIndex(text, "{")
	if idx < 0 {
		return text, ""
	}
	id := text[idx+1:]
	if id == "" || len([]rune(id)) > 5 {
		return text, ""
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			if r < 'A' || r > 'Z' {
				if r < 'a' || r > 'z' {
					return text, ""
				}
			}
		}
	}
	return strings.TrimSpace(text[:idx]), normalizeAPRSMessageID(id)
}

func maidenheadCenter(locator string) (float64, float64, error) {
	locator = strings.ToUpper(strings.TrimSpace(locator))
	if !maidenheadRE.MatchString(locator) {
		return 0, 0, fmt.Errorf("invalid Maidenhead locator: %s", locator)
	}
	lon := -180.0
	lat := -90.0
	lonSize := 20.0
	latSize := 10.0

	for i := 0; i < len(locator); i += 2 {
		switch i {
		case 0:
			lon += float64(locator[i]-'A') * lonSize
			lat += float64(locator[i+1]-'A') * latSize
		case 2, 6:
			lonSize /= 10
			latSize /= 10
			lon += float64(locator[i]-'0') * lonSize
			lat += float64(locator[i+1]-'0') * latSize
		case 4, 8:
			lonSize /= 24
			latSize /= 24
			lon += float64(locator[i]-'A') * lonSize
			lat += float64(locator[i+1]-'A') * latSize
		}
	}
	return lat + latSize/2, lon + lonSize/2, nil
}

func normalizeAPRSMessageID(messageID string) string {
	messageID = strings.TrimSpace(messageID)
	if len(messageID) > 5 {
		messageID = messageID[:5]
	}
	var b strings.Builder
	for _, r := range messageID {
		if (r >= '0' && r <= '9') || (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
			b.WriteRune(r)
		}
	}
	return strings.ToUpper(b.String())
}

func normalizeAPRSCallsign(callsign string) string {
	return strings.ToUpper(strings.TrimSpace(callsign))
}

func validAPRSCallsign(callsign string) bool {
	return aprsCallsignRE.MatchString(normalizeAPRSCallsign(callsign))
}

func validAPRSBaseCallsign(callsign string) bool {
	value := normalizeAPRSCallsign(callsign)
	return value != "" && !strings.Contains(value, "-") && aprsCallsignRE.MatchString(value+"-0")
}

func normalizeAPRSSSID(ssid string) string {
	return strings.TrimSpace(ssid)
}

func validAPRSSSID(ssid string) bool {
	if len(ssid) > 2 {
		return false
	}
	for _, r := range ssid {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func splitAPRSDestination(destination string) (string, string) {
	value := normalizeAPRSCallsign(destination)
	if value == "" {
		return "", "0"
	}
	parts := strings.SplitN(value, "-", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	return parts[0], normalizeAPRSSSID(parts[1])
}

func composeAPRSDestination(callsign, ssid string) string {
	callsign = normalizeAPRSCallsign(callsign)
	if callsign == "" {
		return ""
	}
	ssid = normalizeAPRSSSID(ssid)
	if ssid == "" {
		return callsign
	}
	return fmt.Sprintf("%s-%s", callsign, ssid)
}

func normalizeAPRSStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "sent":
		return "sent"
	case "failed", "failure", "error":
		return "failed"
	case "rejected", "reject", "rej":
		return "rejected"
	default:
		return status
	}
}

func sentAckBadge(item dbAPRSSent) ackBadge {
	if sentRejected(item) {
		return ackBadge{Icon: "✕", Class: "ack-rejected", LabelKey: "web_rejected"}
	}
	if len(item.Parts) == 0 {
		if item.Acked {
			return ackBadge{Icon: "✓", Class: "ack-ok", LabelKey: "web_fully_acked"}
		}
		return ackBadge{LabelKey: "web_no_ack"}
	}
	acked := 0
	for _, part := range item.Parts {
		if part.Acked {
			acked++
		}
	}
	if acked == len(item.Parts) {
		return ackBadge{Icon: "✓", Class: "ack-ok", LabelKey: "web_fully_acked"}
	}
	if acked > 0 {
		return ackBadge{Icon: "?", Class: "ack-partial", LabelKey: "web_partially_acked"}
	}
	return ackBadge{LabelKey: "web_no_ack"}
}

func sentRejected(item dbAPRSSent) bool {
	if normalizeAPRSStatus(item.Status) == "rejected" {
		return true
	}
	for _, part := range item.Parts {
		if normalizeAPRSStatus(part.Status) == "rejected" {
			return true
		}
	}
	return false
}

func (s *server) logAPRSSendResult(source, destination, text, packet, response string, err error) {
	status := "ok"
	if err != nil {
		status = "error: " + err.Error()
	}
	body := strings.TrimRight(response, "\r\n")
	if body == "" {
		body = "<no APRS-IS response>"
	}
	appendLogFile(s.cfg.aprsLog, fmt.Sprintf("%s APRS-IS web-send from=%s to=%s status=%s\nmessage=%s\npacket=%s\naprs-is-response-begin\n%s\naprs-is-response-end\n", now(), source, destination, status, text, packet, body))
}
