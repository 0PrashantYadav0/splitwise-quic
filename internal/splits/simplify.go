package splits

import (
	"container/heap"
	"log"

	"splitwise-quic/internal/models"
)

// acct is a single debtor or creditor node stored in the heap.
type acct struct {
	id   string
	name string
	amt  int64 // always positive; debtors stored as absolute values
}

// acctHeap is a max-heap ordered by amt (largest first).
// Implements heap.Interface so container/heap can drive it.
type acctHeap []acct

func (h acctHeap) Len() int            { return len(h) }
func (h acctHeap) Less(i, j int) bool { return h[i].amt > h[j].amt } // max-heap
func (h acctHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }

func (h *acctHeap) Push(x any) { *h = append(*h, x.(acct)) }
func (h *acctHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// Simplify reduces net balances into a minimal set of transfers using a
// max-heap greedy algorithm (cash-flow minimization).
//
// Algorithm: at each step pop the largest debtor and the largest creditor,
// settle as much as possible between them, then re-insert any remainder.
// This guarantees at most n+m-1 transfers (the theoretical minimum for n
// debtors and m creditors), while always pairing the largest obligations —
// even after partial payments change the relative ordering mid-run.
//
// Complexity: O(n log n) time, O(n) space.
// Balances with Net == 0 are silently skipped.
func Simplify(currency string, balances []models.Balance) []models.Transfer {
	if len(balances) == 0 {
		return nil
	}

	var credSlice, debSlice acctHeap
	var netSum int64

	for _, b := range balances {
		netSum += b.Net
		switch {
		case b.Net > 0:
			credSlice = append(credSlice, acct{b.UserID, b.UserName, b.Net})
		case b.Net < 0:
			debSlice = append(debSlice, acct{b.UserID, b.UserName, -b.Net})
		// Net == 0: skip — this user is already settled
		}
	}

	// A non-zero net sum means the ledger doesn't balance — this is a data
	// integrity violation upstream (expenses + settlements should always net
	// to zero across all participants). Log and continue: the algorithm still
	// produces valid pairwise transfers that clear as much debt as possible.
	if netSum != 0 {
		log.Printf("splits.Simplify: imbalanced ledger for %s (net=%d); possible data integrity issue", currency, netSum)
	}

	// Establish heap invariant in O(n) — faster than n individual Push calls.
	heap.Init(&credSlice)
	heap.Init(&debSlice)

	var transfers []models.Transfer

	for len(credSlice) > 0 && len(debSlice) > 0 {
		// Always pair the largest creditor with the largest debtor.
		c := heap.Pop(&credSlice).(acct) // O(log n)
		d := heap.Pop(&debSlice).(acct)  // O(log n)

		pay := min(d.amt, c.amt) // settle the smaller of the two obligations

		if pay > 0 {
			transfers = append(transfers, models.Transfer{
				FromID:   d.id,
				FromName: d.name,
				ToID:     c.id,
				ToName:   c.name,
				Currency: currency,
				Amount:   pay,
			})
		}

		c.amt -= pay
		d.amt -= pay

		// Re-insert whichever side still has a remaining balance.
		// The heap re-orders it to the correct position in O(log n).
		if c.amt > 0 {
			heap.Push(&credSlice, c)
		}
		if d.amt > 0 {
			heap.Push(&debSlice, d)
		}
	}

	return transfers
}
