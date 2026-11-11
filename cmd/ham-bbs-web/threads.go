package main

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

// Thread writes follow the classic SSH BBS (simple-ham-bbs, bbs/cmd/bbs/mutations.go):
// a reply is inserted by one short immediate transaction that re-reads the
// parent chain, enforces the nesting cap, and allocates the sibling position
// inside the same transaction. Both applications write to the same SQLite file,
// so nothing here may depend on data read before the transaction began.

// errThreadTooDeep is returned when a reply would exceed maxReplyDepth.
var errThreadTooDeep = fmt.Errorf("thread nesting exceeds %d levels", maxReplyDepth)

// messageDepth returns how many levels below the top level id sits: 0 for a
// top-level message, 1 for its reply, and so on. A corrupt parent chain
// terminates instead of looping forever.
func messageDepth(db *gorm.DB, id uint) (int, error) {
	depth := 0
	current := id
	for {
		if depth > maxReplyDepth {
			return depth, errThreadTooDeep
		}
		row := dbMessage{}
		err := db.Select("id", "parent_id").Where("id = ?", current).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return depth, nil
		}
		if err != nil {
			return depth, err
		}
		if row.ParentID == nil {
			return depth, nil
		}
		current = *row.ParentID
		depth++
	}
}

// nextMessagePosition returns the next sibling position inside the caller's
// transaction, so two concurrent replies cannot pick the same slot.
func nextMessagePosition(db *gorm.DB, boardID string, parentID *uint) int {
	var maxPos int
	q := db.Model(&dbMessage{}).Where("board_id = ?", boardID)
	if parentID == nil {
		q = q.Where("parent_id IS NULL")
	} else {
		q = q.Where("parent_id = ?", *parentID)
	}
	q.Select("COALESCE(MAX(position), -1)").Scan(&maxPos)
	return maxPos + 1
}

// ensureBoardExists materialises a missing board row so a message cannot be
// stored where no board page would ever render it. This can only happen when a
// sysop deletes the board between the form render and the post.
func ensureBoardExists(db *gorm.DB, boardID string) error {
	var count int64
	if err := db.Model(&dbBoard{}).Where("id = ?", boardID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	board := dbBoard{ID: boardID, Created: now()}
	if boardID == defaultBoardID {
		board.Name = "General"
		board.Description = "General local messages"
	} else {
		board.Name = boardID
	}
	var maxPos int
	if err := db.Model(&dbBoard{}).Select("COALESCE(MAX(position), -1)").Scan(&maxPos).Error; err != nil {
		return err
	}
	board.Position = maxPos + 1
	return db.Create(&board).Error
}

// createMessage stores one top-level message or reply. Subject, body and author
// are sanitised before they reach the database, the parent chain is checked
// against maxReplyDepth, and the insert is atomic with that check.
func (s *server) createMessage(msg dbMessage) (uint, error) {
	msg.From = sanitizeSingleLineText(msg.From)
	msg.Subject = sanitizeSingleLineText(msg.Subject)
	msg.Body = sanitizeMultilineText(msg.Body)
	msg.Edited = sanitizeSingleLineText(msg.Edited)
	if msg.Created == "" {
		msg.Created = now()
	}
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if msg.ParentID != nil {
			parent := dbMessage{}
			if err := tx.Select("id", "board_id").Where("id = ?", *msg.ParentID).Take(&parent).Error; err != nil {
				return err
			}
			msg.BoardID = parent.BoardID
			depth, err := messageDepth(tx, parent.ID)
			if err != nil {
				return err
			}
			if depth+1 > maxReplyDepth {
				return errThreadTooDeep
			}
		}
		if err := ensureBoardExists(tx, msg.BoardID); err != nil {
			return err
		}
		msg.Position = nextMessagePosition(tx, msg.BoardID, msg.ParentID)
		return tx.Create(&msg).Error
	})
	return msg.ID, err
}

// updateMessage rewrites one message's editable text, sanitised the same way as
// a new post. The SSH BBS offers the same correction path to a sysop; without it
// a typo could only be fixed by deleting the message and its replies.
func (s *server) updateMessage(id uint, subject, body, edited string) error {
	if id == 0 {
		return fmt.Errorf("updateMessage: missing message id")
	}
	return s.db.Model(&dbMessage{}).Where("id = ?", id).Updates(map[string]any{
		"subject": sanitizeSingleLineText(subject),
		"body":    sanitizeMultilineText(body),
		"edited":  sanitizeSingleLineText(edited),
	}).Error
}
