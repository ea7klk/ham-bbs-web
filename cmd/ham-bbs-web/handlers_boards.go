package main

import (
	"gorm.io/gorm"
	"net/http"
	"strconv"
	"strings"
)

func (s *server) boards(w http.ResponseWriter, r *http.Request, user *dbUser) {
	var boards []dbBoard
	s.db.Order("position, id").Find(&boards)
	counts := map[string]int64{}
	for _, b := range boards {
		var count int64
		s.db.Model(&dbMessage{}).Where("board_id = ?", b.ID).Count(&count)
		counts[b.ID] = count
	}
	s.view(w, r, "boards", user, map[string]any{"Boards": boards, "Counts": counts}, "", "")
}

func (s *server) boardShow(w http.ResponseWriter, r *http.Request, user *dbUser) {
	board, err := s.findBoard(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	nodes := s.messageTree(board.ID)
	s.view(w, r, "board", user, map[string]any{"Board": board, "Messages": nodes}, "", "")
}

func (s *server) boardNew(w http.ResponseWriter, r *http.Request, user *dbUser) {
	s.view(w, r, "board_form", user, dbBoard{}, "", "")
}

func (s *server) boardCreate(w http.ResponseWriter, r *http.Request, user *dbUser) {
	name := strings.TrimSpace(r.FormValue("name"))
	board := dbBoard{ID: boardID(name), Name: name, Description: strings.TrimSpace(r.FormValue("description")), Created: now()}
	if board.Name == "" {
		s.view(w, r, "board_form", user, board, "", "Board name is required.")
		return
	}
	var maxPos int
	s.db.Model(&dbBoard{}).Select("COALESCE(MAX(position), -1)").Scan(&maxPos)
	board.Position = maxPos + 1
	if err := s.db.Create(&board).Error; err != nil {
		s.view(w, r, "board_form", user, board, "", err.Error())
		return
	}
	s.logBBSAction(user.Callsign, "web_board_create", "board=%q", board.Name)
	http.Redirect(w, r, "/boards/"+board.ID, http.StatusSeeOther)
}

func (s *server) boardEdit(w http.ResponseWriter, r *http.Request, user *dbUser) {
	board, err := s.findBoard(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.view(w, r, "board_form", user, board, "", "")
}

func (s *server) boardUpdate(w http.ResponseWriter, r *http.Request, user *dbUser) {
	board, err := s.findBoard(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	oldID := board.ID
	name := strings.TrimSpace(r.FormValue("name"))
	newID := boardID(name)
	if name == "" {
		board.Name = name
		s.view(w, r, "board_form", user, board, "", "Board name is required.")
		return
	}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if oldID != newID {
			if err := tx.Model(&dbMessage{}).Where("board_id = ?", oldID).Update("board_id", newID).Error; err != nil {
				return err
			}
			if err := tx.Delete(&dbBoard{}, "id = ?", oldID).Error; err != nil {
				return err
			}
			board.ID = newID
		}
		board.Name = name
		board.Description = strings.TrimSpace(r.FormValue("description"))
		return tx.Save(&board).Error
	})
	if err != nil {
		s.view(w, r, "board_form", user, board, "", err.Error())
		return
	}
	s.logBBSAction(user.Callsign, "web_board_update", "from=%q to=%q", oldID, board.ID)
	http.Redirect(w, r, "/boards/"+board.ID, http.StatusSeeOther)
}

func (s *server) boardDelete(w http.ResponseWriter, r *http.Request, user *dbUser) {
	board, err := s.findBoard(r.PathValue("id"))
	if err == nil {
		var count int64
		s.db.Model(&dbBoard{}).Count(&count)
		if count > 1 {
			s.db.Transaction(func(tx *gorm.DB) error {
				if err := tx.Delete(&dbMessage{}, "board_id = ?", board.ID).Error; err != nil {
					return err
				}
				return tx.Delete(&dbBoard{}, "id = ?", board.ID).Error
			})
			s.logBBSAction(user.Callsign, "web_board_delete", "board=%q", board.Name)
		}
	}
	http.Redirect(w, r, "/boards", http.StatusSeeOther)
}

func (s *server) messagePost(w http.ResponseWriter, r *http.Request, user *dbUser) {
	board, err := s.findBoard(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	msg := dbMessage{BoardID: board.ID, From: user.Callsign, Subject: strings.TrimSpace(r.FormValue("subject")), Body: strings.TrimSpace(r.FormValue("body")), Created: now()}
	if msg.Subject != "" && msg.Body != "" {
		msg.Position = s.nextMessagePosition(board.ID, nil)
		s.db.Create(&msg)
		s.logBBSAction(user.Callsign, "web_message_post", "board=%q subject=%q", board.Name, msg.Subject)
	}
	http.Redirect(w, r, "/boards/"+board.ID, http.StatusSeeOther)
}

func (s *server) messageReply(w http.ResponseWriter, r *http.Request, user *dbUser) {
	parent, err := s.findMessage(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	msg := dbMessage{BoardID: parent.BoardID, ParentID: &parent.ID, From: user.Callsign, Subject: strings.TrimSpace(r.FormValue("subject")), Body: strings.TrimSpace(r.FormValue("body")), Created: now()}
	if msg.Subject != "" && msg.Body != "" {
		msg.Position = s.nextMessagePosition(parent.BoardID, &parent.ID)
		s.db.Create(&msg)
		s.logBBSAction(user.Callsign, "web_message_reply", "board=%q subject=%q", parent.BoardID, msg.Subject)
	}
	http.Redirect(w, r, "/boards/"+parent.BoardID, http.StatusSeeOther)
}

func (s *server) messageDelete(w http.ResponseWriter, r *http.Request, user *dbUser) {
	msg, err := s.findMessage(r.PathValue("id"))
	if err == nil {
		s.deleteMessageTree(msg.ID)
		s.logBBSAction(user.Callsign, "web_message_delete", "board=%q subject=%q", msg.BoardID, msg.Subject)
		http.Redirect(w, r, "/boards/"+msg.BoardID, http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/boards", http.StatusSeeOther)
}

func (s *server) findBoard(id string) (dbBoard, error) {
	var board dbBoard
	return board, s.db.First(&board, "id = ?", id).Error
}

func (s *server) findMessage(rawID string) (dbMessage, error) {
	id, _ := strconv.Atoi(rawID)
	var msg dbMessage
	return msg, s.db.First(&msg, id).Error
}

func (s *server) nextMessagePosition(boardID string, parentID *uint) int {
	var maxPos int
	q := s.db.Model(&dbMessage{}).Where("board_id = ?", boardID)
	if parentID == nil {
		q = q.Where("parent_id IS NULL")
	} else {
		q = q.Where("parent_id = ?", *parentID)
	}
	q.Select("COALESCE(MAX(position), -1)").Scan(&maxPos)
	return maxPos + 1
}

func (s *server) messageTree(boardID string) []messageNode {
	var rows []dbMessage
	s.db.Where("board_id = ?", boardID).Order("parent_id, position, id").Find(&rows)
	byParent := map[uint][]dbMessage{}
	for _, row := range rows {
		parent := uint(0)
		if row.ParentID != nil {
			parent = *row.ParentID
		}
		byParent[parent] = append(byParent[parent], row)
	}
	var build func(uint, int) []messageNode
	build = func(parent uint, depth int) []messageNode {
		out := []messageNode{}
		for _, row := range byParent[parent] {
			out = append(out, messageNode{Row: row, Depth: depth, Replies: build(row.ID, depth+1)})
		}
		return out
	}
	return build(0, 0)
}

func (s *server) deleteMessageTree(id uint) {
	var children []dbMessage
	s.db.Where("parent_id = ?", id).Find(&children)
	for _, child := range children {
		s.deleteMessageTree(child.ID)
	}
	s.db.Delete(&dbMessage{}, id)
}
