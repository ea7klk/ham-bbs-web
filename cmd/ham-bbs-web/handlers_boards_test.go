package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestBoardAndMessageBusinessLogic(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Operator", Email: "a@example.invalid"})
	general := dbBoard{ID: defaultBoardID, Position: 0, Name: "General", Description: "General", Created: now()}
	if err := s.db.Create(&general).Error; err != nil {
		t.Fatal(err)
	}

	created := invokeUserHandler(s.boardCreate, formRequest(http.MethodPost, "/boards/new", url.Values{"name": {" Field Operations "}, "description": {"Local ops"}}), user)
	assertRedirect(t, created, "/boards/field-operations")
	var board dbBoard
	if err := s.db.First(&board, "id = ?", "field-operations").Error; err != nil {
		t.Fatal(err)
	}
	if board.Position != 1 || board.Description != "Local ops" {
		t.Fatalf("created board = %#v", board)
	}
	emptyBoard := invokeUserHandler(s.boardCreate, formRequest(http.MethodPost, "/boards/new", url.Values{"name": {" "}}), user)
	if emptyBoard.Code != http.StatusOK || !strings.Contains(emptyBoard.Body.String(), "required") {
		t.Fatalf("empty board response = %d %q", emptyBoard.Code, emptyBoard.Body.String())
	}

	if got := invokeUserHandler(s.boards, formRequest(http.MethodGet, "/boards", nil), user); got.Code != http.StatusOK {
		t.Fatalf("boards status = %d", got.Code)
	}
	if got := invokeUserHandler(s.boardNew, formRequest(http.MethodGet, "/boards/new", nil), user); got.Code != http.StatusOK {
		t.Fatalf("boardNew status = %d", got.Code)
	}
	show := invokeUserHandler(s.boardShow, setPathValue(formRequest(http.MethodGet, "/boards/field-operations", nil), "id", "field-operations"), user)
	if show.Code != http.StatusOK || !strings.Contains(show.Body.String(), "Field Operations") {
		t.Fatalf("boardShow response = %d %q", show.Code, show.Body.String())
	}
	missingShow := invokeUserHandler(s.boardShow, setPathValue(formRequest(http.MethodGet, "/boards/missing", nil), "id", "missing"), user)
	if missingShow.Code != http.StatusNotFound {
		t.Fatalf("missing boardShow status = %d", missingShow.Code)
	}
	if got := invokeUserHandler(s.boardEdit, setPathValue(formRequest(http.MethodGet, "/boards/field-operations/edit", nil), "id", "field-operations"), user); got.Code != http.StatusOK {
		t.Fatalf("boardEdit status = %d", got.Code)
	}
	if got := invokeUserHandler(s.boardEdit, setPathValue(formRequest(http.MethodGet, "/boards/missing/edit", nil), "id", "missing"), user); got.Code != http.StatusNotFound {
		t.Fatalf("missing boardEdit status = %d", got.Code)
	}

	post := invokeUserHandler(s.messagePost, setPathValue(formRequest(http.MethodPost, "/boards/field-operations/post", url.Values{"subject": {" First "}, "body": {" Root message "}}), "id", "field-operations"), user)
	assertRedirect(t, post, "/boards/field-operations")
	var root dbMessage
	if err := s.db.First(&root, "board_id = ?", "field-operations").Error; err != nil {
		t.Fatal(err)
	}
	if root.Position != 0 || root.Subject != "First" || root.Body != "Root message" {
		t.Fatalf("root message = %#v", root)
	}
	invalidPost := invokeUserHandler(s.messagePost, setPathValue(formRequest(http.MethodPost, "/boards/field-operations/post", url.Values{"subject": {"No body"}}), "id", "field-operations"), user)
	assertRedirect(t, invalidPost, "/boards/field-operations")
	var messageCount int64
	s.db.Model(&dbMessage{}).Where("board_id = ?", "field-operations").Count(&messageCount)
	if messageCount != 1 {
		t.Fatalf("invalid root message was saved; count=%d", messageCount)
	}

	replyForm := url.Values{"subject": {" Re: First "}, "body": {" First reply "}}
	reply := invokeUserHandler(s.messageReply, setPathValue(formRequest(http.MethodPost, "/messages/1/reply", replyForm), "id", "1"), user)
	assertRedirect(t, reply, "/boards/field-operations")
	var firstReply dbMessage
	if err := s.db.Where("parent_id = ?", root.ID).First(&firstReply).Error; err != nil {
		t.Fatal(err)
	}
	secondReply := invokeUserHandler(s.messageReply, setPathValue(formRequest(http.MethodPost, "/messages/1/reply", url.Values{"subject": {"Second"}, "body": {"Second reply"}}), "id", "1"), user)
	assertRedirect(t, secondReply, "/boards/field-operations")
	invalidReply := invokeUserHandler(s.messageReply, setPathValue(formRequest(http.MethodPost, "/messages/1/reply", url.Values{"subject": {"Missing body"}}), "id", "1"), user)
	assertRedirect(t, invalidReply, "/boards/field-operations")
	missingReply := invokeUserHandler(s.messageReply, setPathValue(formRequest(http.MethodPost, "/messages/999/reply", replyForm), "id", "999"), user)
	if missingReply.Code != http.StatusNotFound {
		t.Fatalf("missing messageReply status = %d", missingReply.Code)
	}

	nodes := s.messageTree("field-operations")
	if len(nodes) != 1 || nodes[0].Depth != 0 || len(nodes[0].Replies) != 2 || nodes[0].Replies[0].Depth != 1 {
		t.Fatalf("messageTree() = %#v", nodes)
	}
	if got := s.nextMessagePosition("field-operations", nil); got != 1 {
		t.Fatalf("next root message position = %d, want 1", got)
	}
	if got := s.nextMessagePosition("field-operations", &root.ID); got != 2 {
		t.Fatalf("next reply position = %d, want 2", got)
	}
	if _, err := s.findMessage("not-a-number"); err == nil {
		t.Fatal("invalid message ID unexpectedly found a message")
	}

	sameID := invokeUserHandler(s.boardUpdate, setPathValue(formRequest(http.MethodPost, "/boards/field-operations/edit", url.Values{"name": {"Field Operations"}, "description": {"Updated"}}), "id", "field-operations"), user)
	assertRedirect(t, sameID, "/boards/field-operations")
	emptyUpdate := invokeUserHandler(s.boardUpdate, setPathValue(formRequest(http.MethodPost, "/boards/field-operations/edit", url.Values{"name": {" "}}), "id", "field-operations"), user)
	if emptyUpdate.Code != http.StatusOK || !strings.Contains(emptyUpdate.Body.String(), "required") {
		t.Fatalf("empty board update response = %d %q", emptyUpdate.Code, emptyUpdate.Body.String())
	}
	renamed := invokeUserHandler(s.boardUpdate, setPathValue(formRequest(http.MethodPost, "/boards/field-operations/edit", url.Values{"name": {"Renamed Board"}, "description": {"Renamed"}}), "id", "field-operations"), user)
	assertRedirect(t, renamed, "/boards/renamed-board")
	var movedCount int64
	s.db.Model(&dbMessage{}).Where("board_id = ?", "renamed-board").Count(&movedCount)
	if movedCount != 3 {
		t.Fatalf("renamed board message count = %d, want 3", movedCount)
	}

	secondBoard := dbBoard{ID: "second", Position: 2, Name: "Second", Created: now()}
	if err := s.db.Create(&secondBoard).Error; err != nil {
		t.Fatal(err)
	}
	deleted := invokeUserHandler(s.boardDelete, setPathValue(formRequest(http.MethodPost, "/boards/renamed-board/delete", nil), "id", "renamed-board"), user)
	assertRedirect(t, deleted, "/boards")
	if _, err := s.findBoard("renamed-board"); err == nil {
		t.Fatal("deleted board still exists")
	}
	s.db.Model(&dbMessage{}).Where("board_id = ?", "renamed-board").Count(&movedCount)
	if movedCount != 0 {
		t.Fatalf("deleted board messages = %d, want 0", movedCount)
	}
	secondDeleted := invokeUserHandler(s.boardDelete, setPathValue(formRequest(http.MethodPost, "/boards/second/delete", nil), "id", "second"), user)
	assertRedirect(t, secondDeleted, "/boards")
	protected := invokeUserHandler(s.boardDelete, setPathValue(formRequest(http.MethodPost, "/boards/general/delete", nil), "id", "general"), user)
	assertRedirect(t, protected, "/boards")
	if _, err := s.findBoard("general"); err != nil {
		t.Fatal("last board was deleted")
	}
	missingDelete := invokeUserHandler(s.boardDelete, setPathValue(formRequest(http.MethodPost, "/boards/missing/delete", nil), "id", "missing"), user)
	assertRedirect(t, missingDelete, "/boards")
}

func TestMessageDeleteTree(t *testing.T) {
	s := newTestServer(t)
	user := testUser(t, s, dbUser{Callsign: "EA1ABC", FullName: "Operator", Email: "a@example.invalid"})
	if err := s.db.Create(&dbBoard{ID: "general", Name: "General"}).Error; err != nil {
		t.Fatal(err)
	}
	root := dbMessage{BoardID: "general", Position: 0, From: user.Callsign, Subject: "Root", Body: "body"}
	if err := s.db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	child := dbMessage{BoardID: "general", ParentID: &root.ID, Position: 0, From: user.Callsign, Subject: "Child", Body: "reply"}
	if err := s.db.Create(&child).Error; err != nil {
		t.Fatal(err)
	}
	response := invokeUserHandler(s.messageDelete, setPathValue(formRequest(http.MethodPost, "/messages/1/delete", nil), "id", "1"), user)
	assertRedirect(t, response, "/boards/general")
	var count int64
	s.db.Model(&dbMessage{}).Count(&count)
	if count != 0 {
		t.Fatalf("message tree count after delete = %d", count)
	}
	missing := invokeUserHandler(s.messageDelete, setPathValue(formRequest(http.MethodPost, "/messages/999/delete", nil), "id", "999"), user)
	assertRedirect(t, missing, "/boards")
}
