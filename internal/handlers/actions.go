package handlers

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"splitwise-quic/internal/models"
	"splitwise-quic/internal/realtime"
	"splitwise-quic/internal/splits"
)

func (h *Handlers) createGroup(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	u := userFrom(r)
	g, err := h.store.CreateGroup(name, u.ID)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = h.store.LogActivity(g.ID, u.ID, "created the group")
	http.Redirect(w, r, "/g/"+g.ID, http.StatusSeeOther)
}

func (h *Handlers) addMember(w http.ResponseWriter, r *http.Request) {
	g := h.requireMember(w, r)
	if g == nil {
		return
	}
	uid := r.FormValue("user_id")
	if uid != "" {
		if err := h.store.AddMember(g.ID, uid); err != nil {
			httpError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if added, err := h.store.UserByID(uid); err == nil {
			_ = h.store.LogActivity(g.ID, userFrom(r).ID, "added "+added.Name)
		}
		h.publish(g.ID, "member", "A member was added")
		h.notifyUser(uid, "member", "You were added to \""+g.Name+"\"")
	}
	http.Redirect(w, r, "/g/"+g.ID, http.StatusSeeOther)
}

func (h *Handlers) createExpense(w http.ResponseWriter, r *http.Request) {
	g := h.requireMember(w, r)
	if g == nil {
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
		GroupID:     g.ID,
		PaidBy:      r.FormValue("paid_by"),
		Description: strings.TrimSpace(r.FormValue("description")),
		Amount:      amount,
		Currency:    strings.ToUpper(strings.TrimSpace(r.FormValue("currency"))),
		SplitType:   st,
	}
	if e.Currency == "" {
		e.Currency = "USD"
	}
	if _, err := h.store.CreateExpense(e, inputs); err != nil {
		h.flashRedirect(w, r, friendlySplitErr(err), "/g/"+g.ID)
		return
	}
	// Receipt upload (sent over a QUIC stream when on HTTP/3).
	if fname, err := h.saveReceipt(r, e.ID); err == nil && fname != "" {
		_ = h.store.SetReceipt(g.ID, e.ID, fname)
	}
	if payer, err := h.store.UserByID(e.PaidBy); err == nil {
		_ = h.store.LogActivity(g.ID, userFrom(r).ID,
			fmt.Sprintf("added \"%s\" (%s %s paid by %s)", e.Description, e.Currency, render2(e.Amount), payer.Name))
	}
	h.publish(g.ID, "expense", "New expense: "+e.Description)
	h.notifyMembers(g.ID, userFrom(r).ID, "expense",
		fmt.Sprintf("New expense in \"%s\": %s (%s %s)", g.Name, e.Description, e.Currency, render2(e.Amount)))
	http.Redirect(w, r, "/g/"+g.ID, http.StatusSeeOther)
}

// buildInputs converts form values into split inputs for the chosen type.
func (h *Handlers) buildInputs(r *http.Request, st models.SplitType, included []string) ([]splits.Input, error) {
	inputs := make([]splits.Input, 0, len(included))
	for _, uid := range included {
		raw := r.FormValue("value_" + uid)
		var val int64
		switch st {
		case models.SplitEqual:
			val = 0
		case models.SplitExact:
			v, err := parseMoney(raw)
			if err != nil {
				return nil, fmt.Errorf("invalid exact amount")
			}
			val = v
		case models.SplitPercentage:
			f, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
			if err != nil {
				return nil, fmt.Errorf("invalid percentage")
			}
			val = int64(math.Round(f * 100)) // percent -> basis points
		case models.SplitShares:
			f, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
			if err != nil {
				return nil, fmt.Errorf("invalid share")
			}
			val = int64(math.Round(f))
		default:
			return nil, fmt.Errorf("unknown split type")
		}
		inputs = append(inputs, splits.Input{UserID: uid, Value: val})
	}
	return inputs, nil
}

func (h *Handlers) settle(w http.ResponseWriter, r *http.Request) {
	g := h.requireMember(w, r)
	if g == nil {
		return
	}
	amount, err := parseMoney(r.FormValue("amount"))
	if err != nil || amount <= 0 {
		h.flashRedirect(w, r, "Please enter a valid settlement amount.", "/g/"+g.ID)
		return
	}
	st := &models.Settlement{
		GroupID:  g.ID,
		FromUser: r.FormValue("from_user"),
		ToUser:   r.FormValue("to_user"),
		Amount:   amount,
		Currency: strings.ToUpper(strings.TrimSpace(r.FormValue("currency"))),
	}
	if _, err := h.store.CreateSettlement(st); err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	from, _ := h.store.UserByID(st.FromUser)
	to, _ := h.store.UserByID(st.ToUser)
	if from != nil && to != nil {
		_ = h.store.LogActivity(g.ID, userFrom(r).ID,
			fmt.Sprintf("recorded %s paying %s %s %s", from.Name, to.Name, st.Currency, render2(st.Amount)))
	}
	h.publish(g.ID, "settlement", "A payment was settled")
	if from != nil && to != nil {
		h.notifyUser(st.ToUser, "settlement",
			fmt.Sprintf("%s paid you %s %s in \"%s\"", from.Name, st.Currency, render2(st.Amount), g.Name))
	}
	http.Redirect(w, r, "/g/"+g.ID, http.StatusSeeOther)
}

func (h *Handlers) deleteExpense(w http.ResponseWriter, r *http.Request) {
	g := h.requireMember(w, r)
	if g == nil {
		return
	}
	if err := h.store.DeleteExpense(g.ID, r.PathValue("eid")); err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = h.store.LogActivity(g.ID, userFrom(r).ID, "deleted an expense")
	h.publish(g.ID, "expense", "An expense was deleted")
	http.Redirect(w, r, "/g/"+g.ID, http.StatusSeeOther)
}

// trim is a tiny alias for strings.TrimSpace used across handlers.
func trim(s string) string { return strings.TrimSpace(s) }

// upper trims and upper-cases (used for currency codes).
func upper(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }

// friendlySplitErr turns split-math errors into guidance the user can act on.
func friendlySplitErr(err error) string {
	switch {
	case err == nil:
		return ""
	case strings.Contains(err.Error(), "reconcile"):
		return "The split doesn't add up to the total. For exact amounts they must sum to the total; for percentages they must total 100%."
	case strings.Contains(err.Error(), "exact"):
		return "Enter a valid exact amount for each participant."
	case strings.Contains(err.Error(), "percentage"):
		return "Enter a valid percentage for each participant (they must total 100%)."
	case strings.Contains(err.Error(), "share"):
		return "Enter a valid (non-negative) share for each participant."
	default:
		return "Couldn't save the expense: " + err.Error()
	}
}

func (h *Handlers) publish(groupID, kind, msg string) {
	h.hub.Publish(groupID, realtime.Event{Kind: kind, Message: msg})
}

// notifyMembers sends a personal push notification to every group member
// except the actor who triggered the change.
func (h *Handlers) notifyMembers(groupID, exceptUserID, kind, msg string) {
	members, err := h.store.GroupMembers(groupID)
	if err != nil {
		return
	}
	for _, m := range members {
		if m.ID == exceptUserID {
			continue
		}
		h.hub.Publish(realtime.UserTopic(m.ID), realtime.Event{Kind: kind, Message: msg})
	}
}

// notifyUser sends a personal push notification to a single user.
func (h *Handlers) notifyUser(userID, kind, msg string) {
	h.hub.Publish(realtime.UserTopic(userID), realtime.Event{Kind: kind, Message: msg})
}

// parseMoney converts a decimal string like "12.34" into minor units (1234).
func parseMoney(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty amount")
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return int64(math.Round(f * 100)), nil
}

// render2 formats minor units as a 2-decimal string (mirrors render.Money).
func render2(minor int64) string {
	neg := minor < 0
	if neg {
		minor = -minor
	}
	out := fmt.Sprintf("%d.%02d", minor/100, minor%100)
	if neg {
		return "-" + out
	}
	return out
}
