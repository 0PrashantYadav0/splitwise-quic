package store

import "splitwise-quic/internal/models"

// AddComment stores a new comment on an expense and returns it.
func (s *Store) AddComment(expenseID, userID, body string) (*models.Comment, error) {
	c := &models.Comment{ID: newID(), ExpenseID: expenseID, UserID: userID, Body: body}
	_, err := s.db.Exec(
		`INSERT INTO comments (id, expense_id, user_id, body) VALUES (?, ?, ?, ?)`,
		c.ID, c.ExpenseID, c.UserID, c.Body,
	)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// CommentsFor returns all comments on an expense, oldest first.
func (s *Store) CommentsFor(expenseID string) ([]models.Comment, error) {
	rows, err := s.db.Query(`
		SELECT c.id, c.expense_id, c.user_id, u.name, c.body, c.created_at
		FROM comments c
		JOIN users u ON u.id = c.user_id
		WHERE c.expense_id = ?
		ORDER BY c.created_at ASC`, expenseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Comment
	for rows.Next() {
		var c models.Comment
		if err := rows.Scan(&c.ID, &c.ExpenseID, &c.UserID, &c.UserName, &c.Body, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ExpenseGroup returns the group id that owns an expense (for auth checks).
func (s *Store) ExpenseGroup(expenseID string) (string, error) {
	var gid string
	err := s.db.QueryRow(`SELECT group_id FROM expenses WHERE id = ?`, expenseID).Scan(&gid)
	return gid, err
}
