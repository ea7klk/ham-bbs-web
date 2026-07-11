package main

import (
	"bufio"
	"fmt"
	"gorm.io/gorm"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (s *server) aprs(w http.ResponseWriter, r *http.Request, user *dbUser) {
	var sent []dbAPRSSent
	var received []dbAPRSReceived
	s.db.Preload("Parts", func(db *gorm.DB) *gorm.DB { return db.Order("number") }).Where("user_callsign = ?", user.Callsign).Order("position DESC, id DESC").Limit(20).Find(&sent)
	s.db.Where("user_callsign = ?", user.Callsign).Order("position DESC, id DESC").Limit(50).Find(&received)
	s.view(w, r, "aprs", user, map[string]any{"Sent": sent, "Received": received}, "", "")
}

func (s *server) aprsSentDetail(w http.ResponseWriter, r *http.Request, user *dbUser) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	var msg dbAPRSSent
	err := s.db.Preload("Parts", func(db *gorm.DB) *gorm.DB { return db.Order("number") }).
		Where("id = ? AND user_callsign = ?", id, user.Callsign).
		First(&msg).Error
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.view(w, r, "aprs_sent_detail", user, map[string]any{"Message": msg}, "", "")
}

func (s *server) aprsReceivedDetail(w http.ResponseWriter, r *http.Request, user *dbUser) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	var msg dbAPRSReceived
	err := s.db.Where("id = ? AND user_callsign = ?", id, user.Callsign).First(&msg).Error
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.view(w, r, "aprs_received_detail", user, map[string]any{"Message": msg}, "", "")
}

func (s *server) aprsToggle(w http.ResponseWriter, r *http.Request, user *dbUser) {
	enabled := r.FormValue("enable_aprs") == "true"
	s.db.Model(user).Updates(map[string]any{"enable_aprs": enabled, "last_seen": now()})
	s.logBBSAction(user.Callsign, "web_aprs_toggle", "enabled=%t", enabled)
	http.Redirect(w, r, "/aprs", http.StatusSeeOther)
}

func (s *server) aprsSend(w http.ResponseWriter, r *http.Request, user *dbUser) {
	destination := composeAPRSDestination(r.FormValue("destination"), r.FormValue("destination_ssid"))
	text := r.FormValue("text")
	sent, ok := s.sendAPRSMessage(user.Callsign, destination, text)
	_ = s.addSentRecord(user.Callsign, sent)
	s.logBBSAction(user.Callsign, "web_aprs_send", "to=%q parts=%d ok=%t", destination, len(sent.Parts), ok)
	http.Redirect(w, r, "/aprs", http.StatusSeeOther)
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

func formatAPRSMessagePacket(source, destination, text string) string {
	return fmt.Sprintf("%s>APRS,TCPIP*::%-9s:%s", normalizeAPRSCallsign(source), normalizeAPRSCallsign(destination), text)
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
	return aprsBaseCallsign(callsign) + "-0"
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
