package main

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func seedBoard(t *testing.T, s *server) {
	t.Helper()
	if err := s.db.Create(&dbBoard{ID: defaultBoardID, Position: 0, Name: "General", Created: now()}).Error; err != nil {
		t.Fatal(err)
	}
}

func TestReplyDepthIsCappedAtMaxDepth(t *testing.T) {
	s := newTestServer(t)
	seedBoard(t, s)

	parent, err := s.createMessage(dbMessage{BoardID: defaultBoardID, From: "EA1ABC", Subject: "Root", Body: "Root body"})
	if err != nil {
		t.Fatal(err)
	}
	for depth := 1; depth <= maxReplyDepth; depth++ {
		parent, err = s.createMessage(dbMessage{ParentID: &parent, From: "EA1ABC", Subject: "Re", Body: "reply"})
		if err != nil {
			t.Fatalf("reply at depth %d: %v", depth, err)
		}
	}

	if _, err := s.createMessage(dbMessage{ParentID: &parent, From: "EA1ABC", Subject: "Re", Body: "too deep"}); !errors.Is(err, errThreadTooDeep) {
		t.Fatalf("reply beyond max depth error = %v, want errThreadTooDeep", err)
	}

	var count int64
	s.db.Model(&dbMessage{}).Count(&count)
	if count != int64(maxReplyDepth+1) {
		t.Fatalf("stored messages = %d, want %d", count, maxReplyDepth+1)
	}
}

func TestReplyHandlerReportsThreadTooDeep(t *testing.T) {
	s := newTestServer(t)
	seedBoard(t, s)
	user := testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Operator", Email: "a@example.invalid"})

	parent, err := s.createMessage(dbMessage{BoardID: defaultBoardID, From: user.Callsign, Subject: "Root", Body: "Root body"})
	if err != nil {
		t.Fatal(err)
	}
	for depth := 1; depth < maxReplyDepth; depth++ {
		parent, err = s.createMessage(dbMessage{ParentID: &parent, From: user.Callsign, Subject: "Re", Body: "reply"})
		if err != nil {
			t.Fatalf("reply at depth %d: %v", depth, err)
		}
	}

	// The deepest legal reply still works through the handler.
	deepest, err := s.createMessage(dbMessage{ParentID: &parent, From: user.Callsign, Subject: "Re", Body: "deepest"})
	if err != nil {
		t.Fatal(err)
	}
	var stored dbMessage
	if err := s.db.First(&stored, deepest).Error; err != nil {
		t.Fatal(err)
	}

	form := url.Values{"subject": {"Re: deepest"}, "body": {"one level too far"}}
	response := invokeUserHandler(s.messageReply, setPathValue(formRequest(http.MethodPost, "/messages/x/reply", form), "id", strconv.FormatUint(uint64(deepest), 10)), user)
	if response.Code != http.StatusOK {
		t.Fatalf("too-deep reply status = %d, want %d", response.Code, http.StatusOK)
	}
	if !strings.Contains(response.Body.String(), "thread_too_deep") {
		t.Fatalf("too-deep reply body does not explain the limit: %q", response.Body.String())
	}
	var count int64
	s.db.Model(&dbMessage{}).Count(&count)
	if count != int64(maxReplyDepth+1) {
		t.Fatalf("too-deep reply was stored; count = %d", count)
	}
}

func TestBoardPageHidesReplyFormAtMaxDepth(t *testing.T) {
	s := newTestServer(t)
	seedBoard(t, s)
	user := testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Operator", Email: "a@example.invalid"})

	parent, err := s.createMessage(dbMessage{BoardID: defaultBoardID, From: user.Callsign, Subject: "Root", Body: "Root body"})
	if err != nil {
		t.Fatal(err)
	}
	rootID := parent
	for depth := 1; depth <= maxReplyDepth; depth++ {
		parent, err = s.createMessage(dbMessage{ParentID: &parent, From: user.Callsign, Subject: "Re", Body: "reply"})
		if err != nil {
			t.Fatal(err)
		}
	}

	response := invokeUserHandler(s.boardShow, setPathValue(formRequest(http.MethodGet, "/boards/general", nil), "id", defaultBoardID), user)
	body := response.Body.String()
	if !strings.Contains(body, "/messages/"+strconv.FormatUint(uint64(rootID), 10)+"/reply") {
		t.Fatalf("board page lost the reply form for a legal depth: %q", body)
	}
	if strings.Contains(body, "/messages/"+strconv.FormatUint(uint64(parent), 10)+"/reply") {
		t.Fatalf("board page offered a reply form at max depth")
	}
	if !strings.Contains(body, "thread_too_deep") {
		t.Fatalf("board page does not explain the depth limit")
	}
}

func TestMessageDepthBoundsACorruptParentChain(t *testing.T) {
	s := newTestServer(t)
	seedBoard(t, s)
	row := dbMessage{BoardID: defaultBoardID, From: "EA1ABC", Subject: "Loop", Body: "Loop", Created: now()}
	if err := s.db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.db.Model(&dbMessage{}).Where("id = ?", row.ID).Update("parent_id", row.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := messageDepth(s.db, row.ID); !errors.Is(err, errThreadTooDeep) {
		t.Fatalf("corrupt chain error = %v, want errThreadTooDeep", err)
	}
}

func TestReplyTakesTheParentsBoardAndPosition(t *testing.T) {
	s := newTestServer(t)
	seedBoard(t, s)
	parent, err := s.createMessage(dbMessage{BoardID: defaultBoardID, From: "EA1ABC", Subject: "Root", Body: "Root"})
	if err != nil {
		t.Fatal(err)
	}
	// A reply carrying a different board must still land under its parent.
	if _, err := s.createMessage(dbMessage{BoardID: "somewhere-else", ParentID: &parent, From: "EA1ABC", Subject: "A", Body: "A"}); err != nil {
		t.Fatal(err)
	}
	second, err := s.createMessage(dbMessage{BoardID: "somewhere-else", ParentID: &parent, From: "EA1ABC", Subject: "B", Body: "B"})
	if err != nil {
		t.Fatal(err)
	}
	var stored dbMessage
	if err := s.db.First(&stored, second).Error; err != nil {
		t.Fatal(err)
	}
	if stored.BoardID != defaultBoardID || stored.Position != 1 {
		t.Fatalf("stored reply = %#v", stored)
	}
}

func TestCreateMessageFailsForMissingParent(t *testing.T) {
	s := newTestServer(t)
	seedBoard(t, s)
	missing := uint(999)
	if _, err := s.createMessage(dbMessage{ParentID: &missing, From: "EA1ABC", Subject: "Re", Body: "body"}); err == nil {
		t.Fatal("reply to a missing parent was stored")
	}
}
