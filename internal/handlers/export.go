package handlers

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"sort"
	"time"

	"github.com/go-pdf/fpdf"
)

// exportCSV streams a group's expenses as a CSV (one row per participant share).
func (h *Handlers) exportCSV(w http.ResponseWriter, r *http.Request) {
	g := h.requireMember(w, r)
	if g == nil {
		return
	}
	expenses, err := h.store.Expenses(g.ID)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s-expenses.csv"`, slug(g.Name)))

	cw := csv.NewWriter(w)
	defer cw.Flush()
	_ = cw.Write([]string{"Date", "Description", "Paid By", "Currency", "Total", "Split", "Participant", "Share"})
	for _, e := range expenses {
		for _, sh := range e.Shares {
			_ = cw.Write([]string{
				e.CreatedAt.Format("2006-01-02 15:04"),
				e.Description, e.PaidByName, e.Currency, render2(e.Amount),
				string(e.SplitType), sh.UserName, render2(sh.Amount),
			})
		}
	}
}

// exportPDF renders a one-page PDF summary: expenses, balances, and the
// simplified settle-up plan.
func (h *Handlers) exportPDF(w http.ResponseWriter, r *http.Request) {
	g := h.requireMember(w, r)
	if g == nil {
		return
	}
	expenses, err := h.store.Expenses(g.ID)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	balances, err := h.store.Balances(g.ID)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	transfers, err := h.store.SimplifiedTransfers(g.ID)
	if err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetTitle(g.Name+" — Splitwise-QUIC", false)
	pdf.AddPage()

	pdf.SetFont("Arial", "B", 18)
	pdf.SetTextColor(16, 122, 87)
	pdf.Cell(0, 10, g.Name)
	pdf.Ln(9)
	pdf.SetFont("Arial", "", 9)
	pdf.SetTextColor(120, 120, 120)
	pdf.Cell(0, 5, "Splitwise-QUIC report  -  generated "+time.Now().Format("Jan 2, 2006 15:04"))
	pdf.Ln(10)

	// Expenses table
	section(pdf, "Expenses")
	pdf.SetFont("Arial", "B", 9)
	pdf.SetFillColor(236, 253, 245)
	pdf.SetTextColor(40, 40, 40)
	headers := []struct {
		t string
		w float64
	}{{"Date", 28}, {"Description", 62}, {"Paid by", 40}, {"Cur", 14}, {"Amount", 26}, {"Split", 20}}
	for _, hd := range headers {
		pdf.CellFormat(hd.w, 7, hd.t, "1", 0, "L", true, 0, "")
	}
	pdf.Ln(-1)
	pdf.SetFont("Arial", "", 9)
	for _, e := range expenses {
		pdf.CellFormat(28, 6, e.CreatedAt.Format("2006-01-02"), "1", 0, "L", false, 0, "")
		pdf.CellFormat(62, 6, clip(e.Description, 40), "1", 0, "L", false, 0, "")
		pdf.CellFormat(40, 6, clip(e.PaidByName, 24), "1", 0, "L", false, 0, "")
		pdf.CellFormat(14, 6, e.Currency, "1", 0, "C", false, 0, "")
		pdf.CellFormat(26, 6, render2(e.Amount), "1", 0, "R", false, 0, "")
		pdf.CellFormat(20, 6, string(e.SplitType), "1", 0, "L", false, 0, "")
		pdf.Ln(-1)
	}
	if len(expenses) == 0 {
		pdf.SetTextColor(150, 150, 150)
		pdf.Cell(0, 6, "No expenses recorded.")
		pdf.Ln(6)
	}
	pdf.Ln(4)

	// Balances
	section(pdf, "Balances")
	pdf.SetFont("Arial", "", 10)
	pdf.SetTextColor(40, 40, 40)
	for _, cur := range sortedKeys(balances) {
		pdf.SetFont("Arial", "B", 10)
		pdf.Cell(0, 6, cur)
		pdf.Ln(6)
		pdf.SetFont("Arial", "", 10)
		for _, b := range balances[cur] {
			line := fmt.Sprintf("  %s owes %s", b.UserName, render2(-b.Net))
			if b.Net > 0 {
				line = fmt.Sprintf("  %s gets back %s", b.UserName, render2(b.Net))
			}
			pdf.Cell(0, 6, line)
			pdf.Ln(6)
		}
	}
	pdf.Ln(4)

	// Settle-up plan
	section(pdf, "Simplified settle-up")
	pdf.SetFont("Arial", "", 10)
	any := false
	for _, cur := range sortedKeys(transfers) {
		for _, t := range transfers[cur] {
			any = true
			pdf.Cell(0, 6, fmt.Sprintf("  %s -> %s : %s %s", t.FromName, t.ToName, t.Currency, render2(t.Amount)))
			pdf.Ln(6)
		}
	}
	if !any {
		pdf.SetTextColor(16, 122, 87)
		pdf.Cell(0, 6, "  All settled up!")
		pdf.Ln(6)
	}

	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s-report.pdf"`, slug(g.Name)))
	if err := pdf.Output(w); err != nil {
		httpError(w, err.Error(), http.StatusInternalServerError)
	}
}

func section(pdf *fpdf.Fpdf, title string) {
	pdf.SetFont("Arial", "B", 12)
	pdf.SetTextColor(16, 122, 87)
	pdf.Cell(0, 7, title)
	pdf.Ln(8)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}

func slug(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			out = append(out, r)
		case r == ' ' || r == '-' || r == '_':
			out = append(out, '-')
		}
	}
	if len(out) == 0 {
		return "group"
	}
	return string(out)
}
