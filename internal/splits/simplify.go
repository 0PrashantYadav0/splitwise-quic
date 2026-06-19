package splits

import (
	"sort"

	"splitwise-quic/internal/models"
)

// Simplify reduces a set of net balances (per single currency) into the
// minimum-ish set of transfers using the classic greedy "cash flow
// minimization" heuristic: repeatedly settle the biggest creditor against
// the biggest debtor. It's NP-hard to do optimally, but greedy is what
// Splitwise itself ships and it's near-optimal in practice.
//
// Balances must be net per user: positive => is owed, negative => owes.
func Simplify(currency string, balances []models.Balance) []models.Transfer {
	type acct struct {
		id   string
		name string
		amt  int64
	}
	var creditors, debtors []acct
	for _, b := range balances {
		switch {
		case b.Net > 0:
			creditors = append(creditors, acct{b.UserID, b.UserName, b.Net})
		case b.Net < 0:
			debtors = append(debtors, acct{b.UserID, b.UserName, -b.Net})
		}
	}
	// Largest first => fewer, chunkier transfers.
	sort.SliceStable(creditors, func(i, j int) bool { return creditors[i].amt > creditors[j].amt })
	sort.SliceStable(debtors, func(i, j int) bool { return debtors[i].amt > debtors[j].amt })

	var transfers []models.Transfer
	i, j := 0, 0
	for i < len(debtors) && j < len(creditors) {
		d, c := &debtors[i], &creditors[j]
		pay := min64(d.amt, c.amt)
		if pay > 0 {
			transfers = append(transfers, models.Transfer{
				FromID: d.id, FromName: d.name,
				ToID: c.id, ToName: c.name,
				Currency: currency, Amount: pay,
			})
		}
		d.amt -= pay
		c.amt -= pay
		if d.amt == 0 {
			i++
		}
		if c.amt == 0 {
			j++
		}
	}
	return transfers
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}
