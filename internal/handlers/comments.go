package handlers

import "net/http"

// addComment attaches a comment to an expense and notifies the group.
func (h *Handlers) addComment(w http.ResponseWriter, r *http.Request) {
	g := h.requireMember(w, r)
	if g == nil {
		return
	}
	eid := r.PathValue("eid")
	e, err := h.store.ExpenseByID(g.ID, eid)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	body := trim(r.FormValue("body"))
	if body == "" {
		http.Redirect(w, r, "/g/"+g.ID, http.StatusSeeOther)
		return
	}
	u := userFrom(r)
	if _, err := h.store.AddComment(eid, u.ID, body); err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = h.store.LogActivity(g.ID, u.ID, "commented on \""+e.Description+"\"")
	h.publish(g.ID, "comment", u.Name+" commented on "+e.Description)
	h.notifyMembers(g.ID, u.ID, "comment", u.Name+" commented in \""+g.Name+"\"")
	http.Redirect(w, r, "/g/"+g.ID, http.StatusSeeOther)
}
