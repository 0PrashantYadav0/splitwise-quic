// Package store implements all persistence logic on top of the SQLite DB.
package store

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"splitwise-quic/internal/models"
)

// ErrNotFound is returned when a lookup yields nothing.
var ErrNotFound = errors.New("not found")

// Store is the single entry point for all database operations.
type Store struct {
	db *sql.DB
}

// New wraps an open *sql.DB.
func New(db *sql.DB) *Store { return &Store{db: db} }

func newID() string { return uuid.NewString() }

// --- Users & auth ---------------------------------------------------------

// CreateUser registers a new user with a bcrypt-hashed password.
func (s *Store) CreateUser(email, name, password string) (*models.User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}
	u := &models.User{
		ID:           newID(),
		Email:        email,
		Name:         name,
		PasswordHash: string(hash),
		CreatedAt:    time.Now(),
	}
	_, err = s.db.Exec(
		`INSERT INTO users (id, email, name, password_hash) VALUES (?, ?, ?, ?)`,
		u.ID, u.Email, u.Name, u.PasswordHash,
	)
	if err != nil {
		return nil, err
	}
	return u, nil
}

// Authenticate verifies credentials and returns the user on success.
func (s *Store) Authenticate(email, password string) (*models.User, error) {
	u, err := s.userByEmail(email)
	if err != nil {
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return nil, ErrNotFound
	}
	return u, nil
}

func (s *Store) userByEmail(email string) (*models.User, error) {
	var u models.User
	err := s.db.QueryRow(
		`SELECT id, email, name, password_hash, created_at FROM users WHERE email = ?`, email,
	).Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &u, err
}

// UserByID fetches a single user.
func (s *Store) UserByID(id string) (*models.User, error) {
	var u models.User
	err := s.db.QueryRow(
		`SELECT id, email, name, password_hash, created_at FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.Email, &u.Name, &u.PasswordHash, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &u, err
}

// AllUsers lists every user (used to populate group-member pickers).
func (s *Store) AllUsers() ([]models.User, error) {
	rows, err := s.db.Query(`SELECT id, email, name, created_at FROM users ORDER BY name`)
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

// --- Sessions -------------------------------------------------------------

// CreateSession mints a new opaque session token for a user.
func (s *Store) CreateSession(userID string) (string, error) {
	token := uuid.NewString() + uuid.NewString()
	_, err := s.db.Exec(`INSERT INTO sessions (token, user_id) VALUES (?, ?)`, token, userID)
	return token, err
}

// UserBySession resolves a session token back to its user.
func (s *Store) UserBySession(token string) (*models.User, error) {
	var uid string
	err := s.db.QueryRow(`SELECT user_id FROM sessions WHERE token = ?`, token).Scan(&uid)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.UserByID(uid)
}

// DeleteSession logs a user out.
func (s *Store) DeleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
	return err
}
