package main

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAPRSMessageAndAddressHelpers(t *testing.T) {
	if got := withAPRSMessageID("Hello", " a-1 "); got != "Hello{A1" {
		t.Fatalf("withAPRSMessageID() = %q", got)
	}
	if got := withAPRSMessageID("Hello", ""); got != "Hello" {
		t.Fatalf("withAPRSMessageID(empty) = %q", got)
	}
	if got := withAPRSMessageID("Hello", "bad!"); got != "Hello{BAD" {
		t.Fatalf("withAPRSMessageID(invalid) = %q", got)
	}
	if got := aprsMessageIDs("EA7KLK-0", "EA1ABC-7", 0, time.UnixMilli(1)); got != nil {
		t.Fatalf("aprsMessageIDs(count=0) = %v", got)
	}
	ids := aprsMessageIDs("EA7KLK-0", "EA1ABC-7", 3, time.UnixMilli(1))
	if len(ids) != 3 || ids[0] == ids[1] || len(ids[0]) != 5 {
		t.Fatalf("aprsMessageIDs() = %v", ids)
	}
	for _, id := range ids {
		if !regexp.MustCompile(`^[A-Z0-9]{5}$`).MatchString(id) {
			t.Fatalf("invalid APRS message ID %q", id)
		}
	}
	passcode := aprsPasscode("N0CALL-7")
	if passcode != aprsPasscode("N0CALL") || passcode < 0 || passcode > 32767 {
		t.Fatalf("aprsPasscode() = %d", passcode)
	}
	if got := aprsBaseCallsign(" ea7klk-7 "); got != "EA7KLK" {
		t.Fatalf("aprsBaseCallsign() = %q", got)
	}
	if got := aprsSSID0(" "); got != "" {
		t.Fatalf("aprsSSID0(empty) = %q", got)
	}

	for _, test := range []struct {
		value string
		want  bool
	}{
		{"EA7KLK", true}, {"EA7KLK-0", true}, {"N0CALL-15", true}, {"bad call", false}, {"A", true}, {"EA7KLK-100", false},
	} {
		if got := validAPRSCallsign(test.value); got != test.want {
			t.Fatalf("validAPRSCallsign(%q) = %t, want %t", test.value, got, test.want)
		}
	}
	for _, test := range []struct {
		value string
		want  bool
	}{
		{"EA7KLK", true}, {"EA7KLK-7", false}, {"", false}, {"A", true},
	} {
		if got := validAPRSBaseCallsign(test.value); got != test.want {
			t.Fatalf("validAPRSBaseCallsign(%q) = %t, want %t", test.value, got, test.want)
		}
	}
	for _, test := range []struct {
		value string
		want  bool
	}{
		{"", true}, {"0", true}, {"99", true}, {"100", false}, {"A", false}, {"-1", false},
	} {
		if got := validAPRSSSID(test.value); got != test.want {
			t.Fatalf("validAPRSSSID(%q) = %t, want %t", test.value, got, test.want)
		}
	}
	for _, test := range []struct {
		value, callsign, ssid string
	}{
		{"", "", "0"},
		{"EA1ABC", "EA1ABC", ""},
		{"ea1abc-7", "EA1ABC", "7"},
	} {
		callsign, ssid := splitAPRSDestination(test.value)
		if callsign != test.callsign || ssid != test.ssid {
			t.Fatalf("splitAPRSDestination(%q) = %q/%q, want %q/%q", test.value, callsign, ssid, test.callsign, test.ssid)
		}
	}

	for _, test := range []struct {
		value, want string
	}{
		{"", "sent"}, {" sent ", "sent"}, {"failure", "failed"}, {"error", "failed"}, {"reject", "rejected"}, {"unknown", "unknown"},
	} {
		if got := normalizeAPRSStatus(test.value); got != test.want {
			t.Fatalf("normalizeAPRSStatus(%q) = %q, want %q", test.value, got, test.want)
		}
	}
}

func TestAPRSBodySplittingAndIDs(t *testing.T) {
	if got := splitAPRSMessage(""); len(got) != 1 || got[0] != "" {
		t.Fatalf("splitAPRSMessage(empty) = %q", got)
	}
	if got := splitAPRSMessage(" hello\nworld "); len(got) != 1 || got[0] != "hello world" {
		t.Fatalf("splitAPRSMessage(short) = %q", got)
	}
	long := strings.Repeat("x", aprsMessageLimit+10)
	parts := splitAPRSMessage(long)
	if len(parts) < 2 || !strings.HasPrefix(parts[0], "[1/") {
		t.Fatalf("splitAPRSMessage(long) = %q", parts)
	}
	for _, part := range parts {
		if len([]rune(part)) > aprsMessageLimit {
			t.Fatalf("APRS part exceeds limit: %d", len([]rune(part)))
		}
	}
	if got := splitRunes("abc", 2); !strings.EqualFold(strings.Join(got, "|"), "ab|c") {
		t.Fatalf("splitRunes() = %q", got)
	}
	if got := splitRunes("abc", 10); len(got) != 1 || got[0] != "abc" {
		t.Fatalf("splitRunes(large limit) = %q", got)
	}
	if got := cleanAPRSBody(" a\r\nb\t\x00 "); got != "a b ?" {
		t.Fatalf("cleanAPRSBody() = %q", got)
	}
	for _, test := range []struct {
		value, body, id string
	}{
		{"message", "message", ""},
		{"message{a12", "message", "A12"},
		{"message{a-1", "message{a-1", ""},
		{"message{}", "message{}", ""},
		{"message{123456", "message{123456", ""},
		{"message{123!", "message{123!", ""},
	} {
		body, id := splitAPRSMessageID(test.value)
		if body != test.body || id != test.id {
			t.Fatalf("splitAPRSMessageID(%q) = %q/%q, want %q/%q", test.value, body, id, test.body, test.id)
		}
	}
	if got := normalizeAPRSMessageID(" ab-12!cdef "); got != "AB12" {
		t.Fatalf("normalizeAPRSMessageID() = %q", got)
	}
	if got := aprsListText(" one\ntwo{A12"); got != "one two" {
		t.Fatalf("aprsListText() = %q", got)
	}
}

func TestAPRSPositionAndAckRules(t *testing.T) {
	if got := formatAPRSPosition(-12.5, 34.25, '/', '>', " comment\n"); got != "!1230.00S/03415.00E>comment" {
		t.Fatalf("formatAPRSPosition() = %q", got)
	}
	if _, _, err := maidenheadCenter("invalid"); err == nil {
		t.Fatal("invalid Maidenhead locator was accepted")
	}
	if _, _, err := maidenheadCenter("IM77AH12"); err != nil {
		t.Fatalf("extended Maidenhead locator was rejected: %v", err)
	}
	for _, test := range []struct {
		item  dbAPRSSent
		icon  string
		class string
	}{
		{dbAPRSSent{Status: "rejected"}, "✕", "ack-rejected"},
		{dbAPRSSent{Acked: true}, "✓", "ack-ok"},
		{dbAPRSSent{}, "", ""},
		{dbAPRSSent{Parts: []dbAPRSSentPart{{Acked: true}, {Acked: false}}}, "?", "ack-partial"},
		{dbAPRSSent{Parts: []dbAPRSSentPart{{Acked: true}, {Acked: true}}}, "✓", "ack-ok"},
		{dbAPRSSent{Parts: []dbAPRSSentPart{{Status: "reject"}}}, "✕", "ack-rejected"},
	} {
		badge := sentAckBadge(test.item)
		if badge.Icon != test.icon || badge.Class != test.class {
			t.Fatalf("sentAckBadge(%#v) = %#v, want icon=%q class=%q", test.item, badge, test.icon, test.class)
		}
	}
	if !sentRejected(dbAPRSSent{Status: "rej"}) || sentRejected(dbAPRSSent{Status: "sent"}) {
		t.Fatal("sentRejected() status rules are wrong")
	}
}

func startTestAPRSServer(t *testing.T, response string, packets int) (int, <-chan []string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan []string, 1)
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			result <- []string{"accept error: " + err.Error()}
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		lines := []string{}
		login, err := reader.ReadString('\n')
		if err != nil {
			result <- []string{"login error: " + err.Error()}
			return
		}
		lines = append(lines, strings.TrimRight(login, "\r\n"))
		_, _ = io.WriteString(conn, response)
		for i := 0; i < packets; i++ {
			line, err := reader.ReadString('\n')
			if err != nil {
				lines = append(lines, "packet error: "+err.Error())
				break
			}
			lines = append(lines, strings.TrimRight(line, "\r\n"))
		}
		result <- lines
	}()
	return listener.Addr().(*net.TCPAddr).Port, result
}

func TestAPRSProtocolAndEmptyMessageSending(t *testing.T) {
	s := newTestServer(t)
	response := "# logresp EA7KLK-0 verified, server test\r\n"
	port, lines := startTestAPRSServer(t, response, 1)
	s.cfg.aprsPort = port
	sent, ok := s.sendAPRSMessage("ea7klk-7", "ea1abc-9", "")
	if !ok || sent.Status != "sent" || sent.Text != "" || sent.From != "EA7KLK-0" || sent.To != "EA1ABC-9" || len(sent.Parts) != 1 || sent.Parts[0].Text != "" {
		t.Fatalf("empty APRS send = %#v, ok=%t", sent, ok)
	}
	serverLines := <-lines
	if len(serverLines) != 2 || !strings.HasPrefix(serverLines[0], "user EA7KLK-0 pass ") || !strings.Contains(serverLines[1], "EA7KLK-0>APRS,TCPIP*::EA1ABC-9") {
		t.Fatalf("APRS server received = %q", serverLines)
	}

	port, lines = startTestAPRSServer(t, response, 1)
	s.cfg.aprsPort = port
	parts, ok := s.sendAPRSParts("EA7KLK-0", aprsPasscode("EA7KLK-0"), "EA1ABC-9", []string{"hello"}, []string{"A12"})
	if !ok || len(parts) != 1 || parts[0].Status != "sent" || parts[0].MessageID != "A12" {
		t.Fatalf("sendAPRSParts() = %#v, ok=%t", parts, ok)
	}
	if got := <-lines; len(got) != 2 || !strings.Contains(got[1], "hello{A12") {
		t.Fatalf("sendAPRSParts server lines = %q", got)
	}
	port, lines = startTestAPRSServer(t, "# logresp EA7KLK-0 unverified\r\n", 0)
	s.cfg.aprsPort = port
	if parts, ok := s.sendAPRSParts("EA7KLK-0", aprsPasscode("EA7KLK-0"), "EA1ABC-9", []string{"hello"}, nil); ok || len(parts) != 1 || parts[0].Status != "failed" {
		t.Fatalf("rejected APRS login = %#v, ok=%t", parts, ok)
	}
	<-lines

	s.cfg.aprsPort = 1
	failed, ok := s.sendAPRSMessage("EA7KLK", "EA1ABC", "hello")
	if ok || failed.Status != "failed" || len(failed.Parts) != 1 {
		t.Fatalf("unreachable APRS send = %#v, ok=%t", failed, ok)
	}
	invalidSource, ok := s.sendAPRSMessage("bad call", "EA1ABC", "hello")
	if ok || invalidSource.Parts[0].Detail != "invalid source callsign" {
		t.Fatalf("invalid source APRS send = %#v, ok=%t", invalidSource, ok)
	}
	invalidDestination, ok := s.sendAPRSMessage("EA7KLK", "bad call", "hello")
	if ok || invalidDestination.Parts[0].Detail != "invalid destination callsign" {
		t.Fatalf("invalid destination APRS send = %#v, ok=%t", invalidDestination, ok)
	}
}

func TestAPRSBeaconAndLoginBeacon(t *testing.T) {
	s := newTestServer(t)
	response := "# logresp EA7KLK-0 verified, server test\r\n"
	port, lines := startTestAPRSServer(t, response, 1)
	s.cfg.aprsPort = port
	if err := s.sendAPRSBeacon("EA7KLK-7", 37.3125, -5.9583333333, "Test beacon"); err != nil {
		t.Fatal(err)
	}
	serverLines := <-lines
	if len(serverLines) != 2 || !strings.HasPrefix(serverLines[0], "user EA7KLK-0 pass ") || !strings.Contains(serverLines[1], "EA7KLK-0>APRS,TCPIP*:!3718.75N\\00557.50WmTest beacon") {
		t.Fatalf("beacon server lines = %q", serverLines)
	}
	if err := s.sendAPRSBeacon("bad call", 0, 0, "bad"); err == nil {
		t.Fatal("invalid beacon source was accepted")
	}
	s.cfg.aprsPort = 1
	if err := s.sendAPRSBeacon("EA7KLK", 0, 0, "unreachable"); err == nil {
		t.Fatal("unreachable beacon was accepted")
	}

	port, lines = startTestAPRSServer(t, response, 1)
	s.cfg.aprsPort = port
	s.sendLoginAPRSBeacon("ea7klk-7", dbUser{EnableAPRS: true, Maidenhead: "IM77AH"})
	serverLines = <-lines
	if len(serverLines) != 2 || !strings.HasPrefix(serverLines[0], "user EA7KLK-0 pass ") || !strings.Contains(serverLines[1], "HamNet BBS") {
		t.Fatalf("login beacon server lines = %q", serverLines)
	}
	s.sendLoginAPRSBeacon("EA7KLK", dbUser{EnableAPRS: false, Maidenhead: "IM77AH"})
	s.sendLoginAPRSBeacon("EA7KLK", dbUser{EnableAPRS: true})
	s.sendLoginAPRSBeacon("EA7KLK", dbUser{EnableAPRS: true, Maidenhead: "ZZ99"})
	s.sendLoginAPRSBeacon("bad call", dbUser{EnableAPRS: true, Maidenhead: "IM77AH"})
}

func TestAPRSLoginResponsesAndPacketWriter(t *testing.T) {
	for _, test := range []struct {
		name string
		data string
		want string
		err  bool
	}{
		{"verified", "noise\n# logresp EA7KLK-0 verified\n", "noise\n# logresp EA7KLK-0 verified\n", false},
		{"unverified", "# logresp EA7KLK-0 unverified\n", "# logresp EA7KLK-0 unverified\n", true},
		{"eof", "partial", "", true},
		{"no logresp", "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\n", "one\ntwo\nthree\nfour\nfive\nsix\nseven\neight\n", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := readAPRSISLoginResponse(bufio.NewReader(strings.NewReader(test.data)))
			if got != test.want || (err != nil) != test.err {
				t.Fatalf("readAPRSISLoginResponse() = %q, %v", got, err)
			}
		})
	}
	left, right := net.Pipe()
	result := make(chan string, 1)
	go func() {
		reader := bufio.NewReader(left)
		line, _ := reader.ReadString('\n')
		result <- line
		left.Close()
	}()
	if err := writeAPRSISPacket(right, "packet"); err != nil {
		t.Fatal(err)
	}
	if got := <-result; got != "packet\r\n" {
		t.Fatalf("writeAPRSISPacket() wrote %q", got)
	}
	right.Close()
}

func TestAPRSRecordPersistenceAndDeletion(t *testing.T) {
	s := newTestServer(t)
	sent := sentAPRS{At: "at", From: "EA7KLK-0", To: "EA1ABC-7", Text: "one\ntwo", Status: "failure", Parts: []sentAPRSPart{{Text: "part\none", Status: "reject", Detail: "detail\n"}}}
	if err := s.addSentRecord("ea7klk", sent); err != nil {
		t.Fatal(err)
	}
	if err := s.addSentRecord("EA7KLK", sentAPRS{From: "EA7KLK-0", To: "EA1ABC", Text: "second", Status: "sent"}); err != nil {
		t.Fatal(err)
	}
	var rows []dbAPRSSent
	if err := s.db.Preload("Parts").Order("position").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].Position != 0 || rows[1].Position != 1 || rows[0].Text != "one two" || rows[0].Status != "failed" || len(rows[0].Parts) != 1 || rows[0].Parts[0].Number != 1 || rows[0].Parts[0].Status != "rejected" {
		t.Fatalf("persisted APRS rows = %#v", rows)
	}
	if _, err := s.findSentAPRS(strconv.FormatUint(uint64(rows[0].ID), 10), "EA7KLK"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.findSentAPRS("bad", "EA7KLK"); err == nil {
		t.Fatal("invalid sent APRS ID was accepted")
	}
	if _, err := s.findSentAPRS(strconv.FormatUint(uint64(rows[0].ID), 10), "OTHER"); err == nil {
		t.Fatal("another user's sent APRS message was exposed")
	}
	received := []dbAPRSReceived{{UserCallsign: "EA7KLK", Position: 0, From: "EA1ABC-7", Text: "hello{A1}", Raw: "raw"}, {UserCallsign: "OTHER", Position: 0, From: "EA2XYZ", Text: "private"}}
	if err := s.db.Create(&received).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := s.findReceivedAPRS(strconv.FormatUint(uint64(received[0].ID), 10), "EA7KLK"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.findReceivedAPRS("bad", "EA7KLK"); err == nil {
		t.Fatal("invalid received APRS ID was accepted")
	}
	if _, err := s.findReceivedAPRS(strconv.FormatUint(uint64(received[0].ID), 10), "OTHER"); err == nil {
		t.Fatal("another user's received APRS message was exposed")
	}
	deleted, err := s.deleteSentAPRS("EA7KLK", []uint{rows[0].ID, 999999})
	if err != nil || deleted != 1 {
		t.Fatalf("deleteSentAPRS() = %d, %v", deleted, err)
	}
	var partCount int64
	s.db.Model(&dbAPRSSentPart{}).Where("sent_id = ?", rows[0].ID).Count(&partCount)
	if partCount != 0 {
		t.Fatal("sent APRS parts were not deleted")
	}
	if deleted, err := s.deleteSentAPRS("EA7KLK", nil); err != nil || deleted != 0 {
		t.Fatalf("deleteSentAPRS(empty) = %d, %v", deleted, err)
	}
	if deleted, err := s.deleteReceivedAPRS("EA7KLK", []uint{received[0].ID, 999999}); err != nil || deleted != 1 {
		t.Fatalf("deleteReceivedAPRS() = %d, %v", deleted, err)
	}
	if deleted, err := s.deleteReceivedAPRS("EA7KLK", nil); err != nil || deleted != 0 {
		t.Fatalf("deleteReceivedAPRS(empty) = %d, %v", deleted, err)
	}

	for i := 0; i < sentHistoryLimit+1; i++ {
		if err := s.db.Create(&dbAPRSSent{UserCallsign: "EA7KLK", Position: i, At: "at", Text: "text"}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := s.trimSentRows("EA7KLK"); err != nil {
		t.Fatal(err)
	}
	var kept int64
	s.db.Model(&dbAPRSSent{}).Where("user_callsign = ?", "EA7KLK").Count(&kept)
	if kept != sentHistoryLimit+1-1 {
		t.Fatalf("trimmed sent APRS rows = %d, want %d", kept, sentHistoryLimit)
	}
}

func TestAPRSHandlersAndOwnership(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA7KLK", FullName: "Operator", Email: "a@example.invalid"})
	sent := dbAPRSSent{UserCallsign: user.Callsign, Position: 0, At: "at", From: "EA7KLK-0", To: "EA1ABC-7", Text: "hello{A1}", Status: "sent", Parts: []dbAPRSSentPart{{Number: 1, Text: "hello", Acked: true}}}
	if err := s.db.Create(&sent).Error; err != nil {
		t.Fatal(err)
	}
	received := dbAPRSReceived{UserCallsign: user.Callsign, Position: 0, At: "at", From: "EA1ABC-7", To: "EA7KLK-0", Text: "reply{B2}", Raw: "raw\npacket"}
	if err := s.db.Create(&received).Error; err != nil {
		t.Fatal(err)
	}
	if got := invokeUserHandler(s.aprs, formRequest(http.MethodGet, "/aprs", nil), user); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "/aprs/received") {
		t.Fatalf("aprs overview = %d %q", got.Code, got.Body.String())
	}
	sentPage := invokeUserHandler(s.aprsSent, formRequest(http.MethodGet, "/aprs/sent", nil), user)
	if sentPage.Code != http.StatusOK || !strings.Contains(sentPage.Body.String(), "hello") {
		t.Fatalf("sent APRS page = %d %q", sentPage.Code, sentPage.Body.String())
	}
	receivedPage := invokeUserHandler(s.aprsReceived, formRequest(http.MethodGet, "/aprs/received", nil), user)
	if receivedPage.Code != http.StatusOK || !strings.Contains(receivedPage.Body.String(), "reply") {
		t.Fatalf("received APRS page = %d %q", receivedPage.Code, receivedPage.Body.String())
	}
	sendForm := invokeUserHandler(s.aprsSendForm, formRequest(http.MethodGet, "/aprs/send", nil), user)
	if sendForm.Code != http.StatusOK || !strings.Contains(sendForm.Body.String(), "/aprs/toggle") {
		t.Fatalf("APRS send form = %d %q", sendForm.Code, sendForm.Body.String())
	}
	sentDetail := invokeUserHandler(s.aprsSentDetail, setPathValue(formRequest(http.MethodGet, "/aprs/sent/1", nil), "id", strconv.Itoa(int(sent.ID))), user)
	if sentDetail.Code != http.StatusOK || !strings.Contains(sentDetail.Body.String(), "hello") {
		t.Fatalf("sent detail = %d %q", sentDetail.Code, sentDetail.Body.String())
	}
	receivedDetail := invokeUserHandler(s.aprsReceivedDetail, setPathValue(formRequest(http.MethodGet, "/aprs/received/1", nil), "id", strconv.Itoa(int(received.ID))), user)
	if receivedDetail.Code != http.StatusOK || !strings.Contains(receivedDetail.Body.String(), "reply") || !strings.Contains(receivedDetail.Body.String(), "raw packet") {
		t.Fatalf("received detail = %d %q", receivedDetail.Code, receivedDetail.Body.String())
	}
	replyForm := invokeUserHandler(s.aprsReceivedReplyForm, setPathValue(formRequest(http.MethodGet, "/aprs/received/1/reply", nil), "id", strconv.Itoa(int(received.ID))), user)
	if replyForm.Code != http.StatusOK || !strings.Contains(replyForm.Body.String(), "EA1ABC") {
		t.Fatalf("reply form = %d %q", replyForm.Code, replyForm.Body.String())
	}
	for _, handler := range []func(http.ResponseWriter, *http.Request, *dbUser){s.aprsSentDetail, s.aprsReceivedDetail, s.aprsReceivedReplyForm} {
		missing := invokeUserHandler(handler, setPathValue(formRequest(http.MethodGet, "/aprs/missing", nil), "id", "bad"), user)
		if missing.Code != http.StatusNotFound {
			t.Fatalf("missing APRS handler status = %d", missing.Code)
		}
	}
	if got := invokeUserHandler(s.aprsToggle, formRequest(http.MethodPost, "/aprs/toggle", url.Values{"enable_aprs": {"true"}}), user); got.Code != http.StatusSeeOther {
		t.Fatalf("aprsToggle status = %d", got.Code)
	}
	var savedUser dbUser
	s.db.First(&savedUser, "callsign = ?", user.Callsign)
	if !savedUser.EnableAPRS {
		t.Fatal("APRS toggle did not enable APRS")
	}
	if got := invokeUserHandler(s.aprsSend, formRequest(http.MethodPost, "/aprs/send", url.Values{"destination": {""}, "destination_ssid": {"0"}, "text": {}}), user); got.Code != http.StatusSeeOther {
		t.Fatalf("invalid destination APRS send status = %d", got.Code)
	}
	var sentCount int64
	s.db.Model(&dbAPRSSent{}).Where("user_callsign = ?", user.Callsign).Count(&sentCount)
	if sentCount != 2 {
		t.Fatalf("APRS send record count = %d, want 2", sentCount)
	}
	if got := invokeUserHandler(s.aprsReceivedReply, setPathValue(formRequest(http.MethodPost, "/aprs/received/1/reply", url.Values{"destination": {""}, "destination_ssid": {"0"}, "text": {"reply"}}), "id", strconv.Itoa(int(received.ID))), user); got.Code != http.StatusSeeOther {
		t.Fatalf("received reply status = %d", got.Code)
	}
	badForm := httptest.NewRequest(http.MethodPost, "/aprs/sent/delete", strings.NewReader("%zz"))
	badForm.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if got := invokeUserHandler(s.aprsSentBulkDelete, badForm, user); got.Code != http.StatusBadRequest {
		t.Fatalf("malformed sent bulk form status = %d", got.Code)
	}
	badReceivedForm := httptest.NewRequest(http.MethodPost, "/aprs/received/delete", strings.NewReader("%zz"))
	badReceivedForm.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if got := invokeUserHandler(s.aprsReceivedBulkDelete, badReceivedForm, user); got.Code != http.StatusBadRequest {
		t.Fatalf("malformed received bulk form status = %d", got.Code)
	}

	bulkSent := dbAPRSSent{UserCallsign: user.Callsign, Position: 2, To: "EA2XYZ", Text: "bulk"}
	if err := s.db.Create(&bulkSent).Error; err != nil {
		t.Fatal(err)
	}
	bulkReceived := dbAPRSReceived{UserCallsign: user.Callsign, Position: 2, From: "EA2XYZ", Text: "bulk"}
	if err := s.db.Create(&bulkReceived).Error; err != nil {
		t.Fatal(err)
	}
	bulkDelete := invokeUserHandler(s.aprsSentBulkDelete, formRequest(http.MethodPost, "/aprs/sent/delete", url.Values{"sent_ids": {strconv.Itoa(int(bulkSent.ID)), "bad", "0", strconv.Itoa(int(bulkSent.ID))}}), user)
	assertRedirect(t, bulkDelete, "/aprs/sent")
	bulkReceivedDelete := invokeUserHandler(s.aprsReceivedBulkDelete, formRequest(http.MethodPost, "/aprs/received/delete", url.Values{"received_ids": {strconv.Itoa(int(bulkReceived.ID))}}), user)
	assertRedirect(t, bulkReceivedDelete, "/aprs/received")
	remainingSent := invokeUserHandler(s.aprsSentDelete, setPathValue(formRequest(http.MethodPost, "/aprs/sent/1/delete", nil), "id", strconv.Itoa(int(sent.ID))), user)
	assertRedirect(t, remainingSent, "/aprs/sent")
	remainingReceived := invokeUserHandler(s.aprsReceivedDelete, setPathValue(formRequest(http.MethodPost, "/aprs/received/1/delete", nil), "id", strconv.Itoa(int(received.ID))), user)
	assertRedirect(t, remainingReceived, "/aprs/received")
	if got := invokeUserHandler(s.aprsSentDelete, setPathValue(formRequest(http.MethodPost, "/aprs/sent/1/delete", nil), "id", strconv.Itoa(int(sent.ID))), user); got.Code != http.StatusNotFound {
		t.Fatalf("missing sent delete status = %d", got.Code)
	}
	if got := invokeUserHandler(s.aprsReceivedDelete, setPathValue(formRequest(http.MethodPost, "/aprs/received/1/delete", nil), "id", strconv.Itoa(int(received.ID))), user); got.Code != http.StatusNotFound {
		t.Fatalf("missing received delete status = %d", got.Code)
	}
	if got := invokeUserHandler(s.aprsReceivedReply, setPathValue(formRequest(http.MethodPost, "/aprs/received/1/reply", nil), "id", strconv.Itoa(int(received.ID))), user); got.Code != http.StatusNotFound {
		t.Fatalf("missing received reply status = %d", got.Code)
	}
}

func TestAPRSLogOutput(t *testing.T) {
	s := newTestServer(t)
	s.logAPRSSendResult("EA7KLK-0", "EA1ABC", "text", "packet", "", nil)
	s.logAPRSSendResult("EA7KLK-0", "EA1ABC", "text", "packet", "response\r\n", fmt.Errorf("failed"))
	data, err := os.ReadFile(s.cfg.aprsLog)
	if err != nil {
		t.Fatal(err)
	}
	log := string(data)
	if !strings.Contains(log, "<no APRS-IS response>") || !strings.Contains(log, "status=error: failed") || !strings.Contains(log, "response") {
		t.Fatalf("APRS log = %q", log)
	}
}
