package splits

import (
	"testing"

	"splitwise-quic/internal/models"
)

func sum(shares []models.Share) int64 {
	var t int64
	for _, s := range shares {
		t += s.Amount
	}
	return t
}

func TestEqualSplitNoLostCents(t *testing.T) {
	// 100 cents across 3 people => 34/33/33, summing exactly to 100.
	in := []Input{{UserID: "a"}, {UserID: "b"}, {UserID: "c"}}
	shares, err := Compute(100, models.SplitEqual, in)
	if err != nil {
		t.Fatal(err)
	}
	if got := sum(shares); got != 100 {
		t.Fatalf("shares should sum to 100, got %d", got)
	}
	if shares[0].Amount != 34 {
		t.Fatalf("first person should absorb the extra cent, got %d", shares[0].Amount)
	}
}

func TestExactSplitMustReconcile(t *testing.T) {
	in := []Input{{UserID: "a", Value: 60}, {UserID: "b", Value: 30}}
	if _, err := Compute(100, models.SplitExact, in); err == nil {
		t.Fatal("expected error when exact amounts don't sum to total")
	}
	in2 := []Input{{UserID: "a", Value: 70}, {UserID: "b", Value: 30}}
	if _, err := Compute(100, models.SplitExact, in2); err != nil {
		t.Fatalf("valid exact split rejected: %v", err)
	}
}

func TestPercentageBasisPoints(t *testing.T) {
	// 33.33% / 33.33% / 33.34% (in basis points) of 100 => sums to 100.
	in := []Input{
		{UserID: "a", Value: 3333},
		{UserID: "b", Value: 3333},
		{UserID: "c", Value: 3334},
	}
	shares, err := Compute(100, models.SplitPercentage, in)
	if err != nil {
		t.Fatal(err)
	}
	if got := sum(shares); got != 100 {
		t.Fatalf("percentage shares should sum to 100, got %d", got)
	}
}

func TestPercentageMustTotal100(t *testing.T) {
	in := []Input{{UserID: "a", Value: 5000}, {UserID: "b", Value: 4000}}
	if _, err := Compute(100, models.SplitPercentage, in); err == nil {
		t.Fatal("expected error when percentages don't total 100")
	}
}

func TestSharesProportional(t *testing.T) {
	// 2:1 shares of 90 => 60/30.
	in := []Input{{UserID: "a", Value: 2}, {UserID: "b", Value: 1}}
	shares, err := Compute(90, models.SplitShares, in)
	if err != nil {
		t.Fatal(err)
	}
	if sum(shares) != 90 {
		t.Fatalf("share split should sum to 90, got %d", sum(shares))
	}
	if shares[0].Amount != 60 || shares[1].Amount != 30 {
		t.Fatalf("expected 60/30, got %d/%d", shares[0].Amount, shares[1].Amount)
	}
}

func TestSimplifyMinimizesTransfers(t *testing.T) {
	// A is owed 100, B owes 60, C owes 40 => 2 transfers, all to A.
	balances := []models.Balance{
		{UserID: "a", UserName: "A", Net: 100},
		{UserID: "b", UserName: "B", Net: -60},
		{UserID: "c", UserName: "C", Net: -40},
	}
	tx := Simplify("USD", balances)
	if len(tx) != 2 {
		t.Fatalf("expected 2 transfers, got %d", len(tx))
	}
	var total int64
	for _, x := range tx {
		if x.ToID != "a" {
			t.Fatalf("everyone should pay A, got %s", x.ToID)
		}
		total += x.Amount
	}
	if total != 100 {
		t.Fatalf("transfers should total 100, got %d", total)
	}
}
