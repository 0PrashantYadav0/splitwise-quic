// Package models holds the core domain types for the Splitwise clone.
package models

import "time"

// User is a registered person who can belong to groups and pay/owe money.
type User struct {
	ID           string    `json:"id"`
	Email        string    `json:"email"`
	Name         string    `json:"name"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// Group is a collection of users who share expenses (a trip, household, etc.).
type Group struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	OwnerID   string    `json:"owner_id"`
	CreatedAt time.Time `json:"created_at"`
	Members   []User    `json:"members,omitempty"`
}

// SplitType describes how an expense is divided among participants.
type SplitType string

const (
	SplitEqual      SplitType = "equal"      // divided evenly
	SplitExact      SplitType = "exact"      // explicit amounts per person
	SplitPercentage SplitType = "percentage" // explicit percentages per person
	SplitShares     SplitType = "shares"     // weighted shares per person
)

// Expense is a single charge paid by one user, split among several.
type Expense struct {
	ID          string    `json:"id"`
	GroupID     string    `json:"group_id"`
	PaidBy      string    `json:"paid_by"`
	Description string    `json:"description"`
	Amount      int64     `json:"amount"` // minor units (cents), avoids float drift
	Currency    string    `json:"currency"`
	SplitType   SplitType `json:"split_type"`
	ReceiptPath string    `json:"receipt_path,omitempty"` // uploaded receipt image, if any
	CreatedAt   time.Time `json:"created_at"`
	Shares      []Share   `json:"shares,omitempty"`
	Comments    []Comment `json:"comments,omitempty"`
	PaidByName  string    `json:"paid_by_name,omitempty"`
}

// Comment is a free-text note attached to an expense.
type Comment struct {
	ID        string    `json:"id"`
	ExpenseID string    `json:"expense_id"`
	UserID    string    `json:"user_id"`
	UserName  string    `json:"user_name,omitempty"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

// Share is one participant's portion of an expense (in minor units).
type Share struct {
	ExpenseID string `json:"expense_id"`
	UserID    string `json:"user_id"`
	UserName  string `json:"user_name,omitempty"`
	Amount    int64  `json:"amount"` // what this user owes for the expense
}

// Settlement is a direct payment from one user to another to clear debt.
type Settlement struct {
	ID        string    `json:"id"`
	GroupID   string    `json:"group_id"`
	FromUser  string    `json:"from_user"`
	ToUser    string    `json:"to_user"`
	Amount    int64     `json:"amount"`
	Currency  string    `json:"currency"`
	CreatedAt time.Time `json:"created_at"`
}

// Activity is an audit/feed entry for a group.
type Activity struct {
	ID        string    `json:"id"`
	GroupID   string    `json:"group_id"`
	ActorID   string    `json:"actor_id"`
	ActorName string    `json:"actor_name"`
	Verb      string    `json:"verb"` // human-readable description
	CreatedAt time.Time `json:"created_at"`
}

// Balance is a net position for a single user in a single currency.
type Balance struct {
	UserID   string `json:"user_id"`
	UserName string `json:"user_name"`
	Currency string `json:"currency"`
	Net      int64  `json:"net"` // positive => owed money, negative => owes money
}

// Transfer is a simplified "who pays whom" instruction after debt minimization.
type Transfer struct {
	FromID   string `json:"from_id"`
	FromName string `json:"from_name"`
	ToID     string `json:"to_id"`
	ToName   string `json:"to_name"`
	Currency string `json:"currency"`
	Amount   int64  `json:"amount"`
}
