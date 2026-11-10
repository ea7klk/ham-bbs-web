package main

// Read paths. Text stored by an older client, or by the SSH BBS before its
// sanitising rule existed, is cleaned here as well as on write, so neither
// surface can display a stored terminal sequence.

// loadBoards reads every board and cleans text written before the sanitising
// rule existed, or by a client that bypassed it.
func (s *server) loadBoards() []dbBoard {
	var boards []dbBoard
	s.db.Order("position, id").Find(&boards)
	for i := range boards {
		boards[i].Name = sanitizeSingleLineText(boards[i].Name)
		boards[i].Description = sanitizeSingleLineText(boards[i].Description)
	}
	return boards
}

// loadBulletins reads every bulletin with the same cleanup as loadBoards.
func (s *server) loadBulletins() []dbBulletin {
	var items []dbBulletin
	s.db.Order("position, id").Find(&items)
	for i := range items {
		items[i].Title = sanitizeSingleLineText(items[i].Title)
		items[i].Body = sanitizeMultilineText(items[i].Body)
		items[i].From = sanitizeSingleLineText(items[i].From)
		items[i].Updated = sanitizeSingleLineText(items[i].Updated)
	}
	return items
}

func (s *server) messageTree(boardID string) []messageNode {
	var rows []dbMessage
	s.db.Where("board_id = ?", boardID).Order("parent_id, position, id").Find(&rows)
	for i := range rows {
		rows[i].From = sanitizeSingleLineText(rows[i].From)
		rows[i].Subject = sanitizeSingleLineText(rows[i].Subject)
		rows[i].Body = sanitizeMultilineText(rows[i].Body)
		rows[i].Edited = sanitizeSingleLineText(rows[i].Edited)
	}
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
