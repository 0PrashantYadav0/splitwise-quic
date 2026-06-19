package store

import (
	"splitwise-quic/internal/models"
	"splitwise-quic/internal/splits"
)

// CreateExpense persists an expense and its computed shares atomically.
func (s *Store) CreateExpense(e *models.Expense, inputs []splits.Input) (*models.Expense, error) {
	shares, err := splits.Compute(e.Amount, e.SplitType, inputs)
	if err != nil {
		return nil, err
	}
	e.ID = newID()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(`
		INSERT INTO expenses (id, group_id, paid_by, description, amount, currency, split_type)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.GroupID, e.PaidBy, e.Description, e.Amount, e.Currency, e.SplitType,
	); err != nil {
		return nil, err
	}
	for _, sh := range shares {
		if _, err := tx.Exec(
			`INSERT INTO expense_shares (expense_id, user_id, amount) VALUES (?, ?, ?)`,
			e.ID, sh.UserID, sh.Amount,
		); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	e.Shares = shares
	return e, nil
}

// Expenses lists a group's expenses (most recent first) with payer names + shares.
func (s *Store) Expenses(groupID string) ([]models.Expense, error) {
	rows, err := s.db.Query(`
		SELECT e.id, e.group_id, e.paid_by, u.name, e.description, e.amount,
		       e.currency, e.split_type, e.created_at
		FROM expenses e
		JOIN users u ON u.id = e.paid_by
		WHERE e.group_id = ?
		ORDER BY e.created_at DESC`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Expense
	for rows.Next() {
		var e models.Expense
		if err := rows.Scan(&e.ID, &e.GroupID, &e.PaidBy, &e.PaidByName, &e.Description,
			&e.Amount, &e.Currency, &e.SplitType, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if out[i].Shares, err = s.expenseShares(out[i].ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) expenseShares(expenseID string) ([]models.Share, error) {
	rows, err := s.db.Query(`
		SELECT es.expense_id, es.user_id, u.name, es.amount
		FROM expense_shares es
		JOIN users u ON u.id = es.user_id
		WHERE es.expense_id = ?`, expenseID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Share
	for rows.Next() {
		var sh models.Share
		if err := rows.Scan(&sh.ExpenseID, &sh.UserID, &sh.UserName, &sh.Amount); err != nil {
			return nil, err
		}
		out = append(out, sh)
	}
	return out, rows.Err()
}

// DeleteExpense removes an expense (shares cascade).
func (s *Store) DeleteExpense(groupID, expenseID string) error {
	_, err := s.db.Exec(`DELETE FROM expenses WHERE id = ? AND group_id = ?`, expenseID, groupID)
	return err
}

// --- Settlements ----------------------------------------------------------

// CreateSettlement records a direct payment between two members.
func (s *Store) CreateSettlement(st *models.Settlement) (*models.Settlement, error) {
	st.ID = newID()
	_, err := s.db.Exec(`
		INSERT INTO settlements (id, group_id, from_user, to_user, amount, currency)
		VALUES (?, ?, ?, ?, ?, ?)`,
		st.ID, st.GroupID, st.FromUser, st.ToUser, st.Amount, st.Currency,
	)
	if err != nil {
		return nil, err
	}
	return st, nil
}

// Settlements lists a group's recorded payments.
func (s *Store) Settlements(groupID string) ([]models.Settlement, error) {
	rows, err := s.db.Query(`
		SELECT id, group_id, from_user, to_user, amount, currency, created_at
		FROM settlements WHERE group_id = ? ORDER BY created_at DESC`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Settlement
	for rows.Next() {
		var st models.Settlement
		if err := rows.Scan(&st.ID, &st.GroupID, &st.FromUser, &st.ToUser,
			&st.Amount, &st.Currency, &st.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, rows.Err()
}
