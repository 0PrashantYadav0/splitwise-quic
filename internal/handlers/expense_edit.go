package handlers

import (
	"fmt"
	"net/http"

	"splitwise-quic/internal/models"
)

// expenseFormView drives the shared create/edit expense form template.
type expenseFormView struct {
	Mode        string // "create" or "edit"
	Action      string // form POST target
	Group       *models.Group
	Expense     *models.Expense   // nil in create mode
	IncludedSet map[string]bool   // member id -> checked
	ValueOf     map[string]string // member id -> prefilled value string
}

// newCreateForm builds the form view for adding a new expense.
func newCreateForm(g *models.Group) expenseFormView {
	inc := make(map[string]bool, len(g.Members))
	val := make(map[string]string, len(g.Members))
	for _, m := range g.Members {
		inc[m.ID] = true
		val[m.ID] = "0"
	}
	return expenseFormView{
		Mode:        "create",
		Action:      "/g/" + g.ID + "/expenses",
		Group:       g,
		IncludedSet: inc,
		ValueOf:     val,
	}
}

// newEditForm builds the form view for editing an existing expense.
func newEditForm(g *models.Group, e *models.Expense) expenseFormView {
	inc := make(map[string]bool, len(g.Members))
	val := make(map[string]string, len(g.Members))
	for _, m := range g.Members {
		val[m.ID] = "0"
	}
	for _, sh := range e.Shares {
		inc[sh.UserID] = true
		val[sh.UserID] = render2(sh.Amount) // perfect for "exact"; a sane default otherwise
	}
	return expenseFormView{
		Mode:        "edit",
		Action:      fmt.Sprintf("/g/%s/expenses/%s", g.ID, e.ID),
		Group:       g,
		Expense:     e,
		IncludedSet: inc,
		ValueOf:     val,
	}
}

// createExpenseForm returns the blank add-expense card (used to cancel an edit).
func (h *Handlers) createExpenseForm(w http.ResponseWriter, r *http.Request) {
	g := h.requireMember(w, r)
	if g == nil {
		return
	}
	_ = h.render.Partial(w, "expense_form_card", newCreateForm(g))
}

// editExpenseForm returns the add-expense card pre-filled for editing.
func (h *Handlers) editExpenseForm(w http.ResponseWriter, r *http.Request) {
	g := h.requireMember(w, r)
	if g == nil {
		return
	}
	e, err := h.store.ExpenseByID(g.ID, r.PathValue("eid"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	_ = h.render.Partial(w, "expense_form_card", newEditForm(g, e))
}

// updateExpense applies edits to an existing expense (and optional new receipt).
func (h *Handlers) updateExpense(w http.ResponseWriter, r *http.Request) {
	g := h.requireMember(w, r)
	if g == nil {
		return
	}
	eid := r.PathValue("eid")
	if _, err := h.store.ExpenseByID(g.ID, eid); err != nil {
		http.NotFound(w, r)
		return
	}
	if err := parseAnyForm(r); err != nil {
		h.flashRedirect(w, r, "Could not read the form: "+err.Error(), "/g/"+g.ID)
		return
	}
	amount, err := parseMoney(r.FormValue("amount"))
	if err != nil || amount <= 0 {
		h.flashRedirect(w, r, "Please enter a valid amount greater than zero.", "/g/"+g.ID)
		return
	}
	st := models.SplitType(r.FormValue("split_type"))
	included := r.Form["include"]
	if len(included) == 0 {
		h.flashRedirect(w, r, "Select at least one participant for the split.", "/g/"+g.ID)
		return
	}
	inputs, err := h.buildInputs(r, st, included)
	if err != nil {
		h.flashRedirect(w, r, friendlySplitErr(err), "/g/"+g.ID)
		return
	}

	e := &models.Expense{
		ID:          eid,
		GroupID:     g.ID,
		PaidBy:      r.FormValue("paid_by"),
		Description: trim(r.FormValue("description")),
		Amount:      amount,
		Currency:    upper(r.FormValue("currency")),
		SplitType:   st,
	}
	if e.Currency == "" {
		e.Currency = "USD"
	}
	// A newly uploaded receipt replaces the old one (same name => overwrite).
	if fname, ferr := h.saveReceipt(r, eid); ferr == nil && fname != "" {
		e.ReceiptPath = fname
	}
	if _, err := h.store.UpdateExpense(e, inputs); err != nil {
		h.flashRedirect(w, r, friendlySplitErr(err), "/g/"+g.ID)
		return
	}
	_ = h.store.LogActivity(g.ID, userFrom(r).ID, fmt.Sprintf("edited \"%s\"", e.Description))
	h.publish(g.ID, "expense", "Expense updated: "+e.Description)
	h.notifyMembers(g.ID, userFrom(r).ID, "expense", fmt.Sprintf("Expense updated in \"%s\": %s", g.Name, e.Description))
	http.Redirect(w, r, "/g/"+g.ID, http.StatusSeeOther)
}
