package store

import (
	"database/sql"
	"errors"

	"splitwise-quic/internal/models"
)

// --- Groups ---------------------------------------------------------------

// CreateGroup creates a group and adds the owner as the first member.
func (s *Store) CreateGroup(name, ownerID string) (*models.Group, error) {
	g := &models.Group{ID: newID(), Name: name, OwnerID: ownerID}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit
	if _, err := tx.Exec(
		`INSERT INTO groups (id, name, owner_id) VALUES (?, ?, ?)`, g.ID, g.Name, g.OwnerID,
	); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(
		`INSERT INTO group_members (group_id, user_id) VALUES (?, ?)`, g.ID, ownerID,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return g, nil
}

// AddMember adds a user to a group (idempotent).
func (s *Store) AddMember(groupID, userID string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO group_members (group_id, user_id) VALUES (?, ?)`, groupID, userID,
	)
	return err
}

// IsMember reports whether a user belongs to a group (authorization check).
func (s *Store) IsMember(groupID, userID string) (bool, error) {
	var x int
	err := s.db.QueryRow(
		`SELECT 1 FROM group_members WHERE group_id = ? AND user_id = ?`, groupID, userID,
	).Scan(&x)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// GroupsForUser lists every group a user belongs to.
func (s *Store) GroupsForUser(userID string) ([]models.Group, error) {
	rows, err := s.db.Query(`
		SELECT g.id, g.name, g.owner_id, g.created_at
		FROM groups g
		JOIN group_members m ON m.group_id = g.id
		WHERE m.user_id = ?
		ORDER BY g.created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Group
	for rows.Next() {
		var g models.Group
		if err := rows.Scan(&g.ID, &g.Name, &g.OwnerID, &g.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// GroupByID returns a group with its members loaded.
func (s *Store) GroupByID(id string) (*models.Group, error) {
	var g models.Group
	err := s.db.QueryRow(
		`SELECT id, name, owner_id, created_at FROM groups WHERE id = ?`, id,
	).Scan(&g.ID, &g.Name, &g.OwnerID, &g.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	g.Members, err = s.GroupMembers(id)
	return &g, err
}

// GroupMembers lists the users in a group.
func (s *Store) GroupMembers(groupID string) ([]models.User, error) {
	rows, err := s.db.Query(`
		SELECT u.id, u.email, u.name, u.created_at
		FROM users u
		JOIN group_members m ON m.user_id = u.id
		WHERE m.group_id = ?
		ORDER BY u.name`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

// --- Activity feed --------------------------------------------------------

// LogActivity appends a human-readable entry to a group's feed.
func (s *Store) LogActivity(groupID, actorID, verb string) error {
	_, err := s.db.Exec(
		`INSERT INTO activities (id, group_id, actor_id, verb) VALUES (?, ?, ?, ?)`,
		newID(), groupID, actorID, verb,
	)
	return err
}

// Activities returns the most recent feed entries for a group.
func (s *Store) Activities(groupID string, limit int) ([]models.Activity, error) {
	rows, err := s.db.Query(`
		SELECT a.id, a.group_id, a.actor_id, u.name, a.verb, a.created_at
		FROM activities a
		JOIN users u ON u.id = a.actor_id
		WHERE a.group_id = ?
		ORDER BY a.created_at DESC
		LIMIT ?`, groupID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.Activity
	for rows.Next() {
		var a models.Activity
		if err := rows.Scan(&a.ID, &a.GroupID, &a.ActorID, &a.ActorName, &a.Verb, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
