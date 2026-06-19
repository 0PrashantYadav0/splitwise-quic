package handlers

import (
	"net/http"

	"splitwise-quic/internal/models"
)

type dashboardView struct {
	view
	Groups []models.Group
}

func (h *Handlers) dashboard(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	groups, err := h.store.GroupsForUser(u.ID)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := dashboardView{view: h.base(w, r, "Dashboard"), Groups: groups}
	if err := h.render.Page(w, "dashboard", data); err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
	}
}

type groupView struct {
	view
	Group    *models.Group
	AllUsers []models.User
	Form     expenseFormView
}

// requireMember loads the group and verifies the current user belongs to it.
// Returns nil group (and writes the response) when access is denied.
func (h *Handlers) requireMember(w http.ResponseWriter, r *http.Request) *models.Group {
	gid := r.PathValue("id")
	u := userFrom(r)
	ok, err := h.store.IsMember(gid, u.ID)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return nil
	}
	if !ok {
		httpError(w, "not a member of this group", http.StatusForbidden)
		return nil
	}
	g, err := h.store.GroupByID(gid)
	if err != nil {
		httpError(w, "group not found", http.StatusNotFound)
		return nil
	}
	return g
}

func (h *Handlers) groupPage(w http.ResponseWriter, r *http.Request) {
	g := h.requireMember(w, r)
	if g == nil {
		return
	}
	all, err := h.store.AllUsers()
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	data := groupView{view: h.base(w, r, g.Name), Group: g, AllUsers: all, Form: newCreateForm(g)}
	if err := h.render.Page(w, "group", data); err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
	}
}
