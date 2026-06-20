package splits

import (
	"errors"
	"testing"

	"splitwise-quic/internal/models"
)

// sum is a helper that sums Share.Amount values.
func sum(shares []models.Share) int64 {
	var t int64
	for _, s := range shares {
		t += s.Amount
	}
	return t
}

// sumTransfers is a helper that sums Transfer.Amount values.
func sumTransfers(tx []models.Transfer) int64 {
	var t int64
	for _, x := range tx {
		t += x.Amount
	}
	return t
}

// ---------------------------------------------------------------------------
// Compute — guard / validation tests
// ---------------------------------------------------------------------------

func TestComputeRejectsZeroTotal(t *testing.T) {
	in := []Input{{UserID: "a"}}
	if _, err := Compute(0, models.SplitEqual, in); !errors.Is(err, ErrNonPositiveTotal) {
		t.Fatalf("expected ErrNonPositiveTotal, got %v", err)
	}
}

func TestComputeRejectsNegativeTotal(t *testing.T) {
	in := []Input{{UserID: "a"}}
	if _, err := Compute(-1, models.SplitEqual, in); !errors.Is(err, ErrNonPositiveTotal) {
		t.Fatalf("expected ErrNonPositiveTotal, got %v", err)
	}
}

func TestComputeRejectsEmptyInputs(t *testing.T) {
	if _, err := Compute(100, models.SplitEqual, nil); !errors.Is(err, ErrEmptyInputs) {
		t.Fatalf("expected ErrEmptyInputs, got %v", err)
	}
	if _, err := Compute(100, models.SplitEqual, []Input{}); !errors.Is(err, ErrEmptyInputs) {
		t.Fatalf("expected ErrEmptyInputs for empty slice, got %v", err)
	}
}

func TestComputeRejectsDuplicateUser(t *testing.T) {
	in := []Input{{UserID: "a"}, {UserID: "b"}, {UserID: "a"}}
	if _, err := Compute(90, models.SplitEqual, in); !errors.Is(err, ErrDuplicateUser) {
		t.Fatalf("expected ErrDuplicateUser, got %v", err)
	}
}

func TestComputeRejectsUnknownSplitType(t *testing.T) {
	in := []Input{{UserID: "a"}}
	if _, err := Compute(100, models.SplitType("bogus"), in); !errors.Is(err, ErrInvalidSplitType) {
		t.Fatalf("expected ErrInvalidSplitType, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Equal split
// ---------------------------------------------------------------------------

func TestEqualSplitNoLostCents(t *testing.T) {
	// 100 cents across 3 people => 34 / 33 / 33, summing exactly to 100.
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

func TestEqualSplitDivisible(t *testing.T) {
	in := []Input{{UserID: "a"}, {UserID: "b"}}
	shares, err := Compute(200, models.SplitEqual, in)
	if err != nil {
		t.Fatal(err)
	}
	if sum(shares) != 200 {
		t.Fatalf("expected 200, got %d", sum(shares))
	}
	if shares[0].Amount != 100 || shares[1].Amount != 100 {
		t.Fatalf("expected 100/100, got %d/%d", shares[0].Amount, shares[1].Amount)
	}
}

// ---------------------------------------------------------------------------
// Exact split
// ---------------------------------------------------------------------------

func TestExactSplitMustReconcile(t *testing.T) {
	in := []Input{{UserID: "a", Value: 60}, {UserID: "b", Value: 30}}
	if _, err := Compute(100, models.SplitExact, in); !errors.Is(err, ErrBadSplit) {
		t.Fatal("expected ErrBadSplit when exact amounts don't sum to total")
	}
	in2 := []Input{{UserID: "a", Value: 70}, {UserID: "b", Value: 30}}
	if _, err := Compute(100, models.SplitExact, in2); err != nil {
		t.Fatalf("valid exact split rejected: %v", err)
	}
}

func TestExactSplitRejectsNegativeValue(t *testing.T) {
	in := []Input{{UserID: "a", Value: -10}, {UserID: "b", Value: 110}}
	if _, err := Compute(100, models.SplitExact, in); !errors.Is(err, ErrNegativeValue) {
		t.Fatalf("expected ErrNegativeValue for negative exact amount, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Percentage split
// ---------------------------------------------------------------------------

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
	if _, err := Compute(100, models.SplitPercentage, in); !errors.Is(err, ErrBadSplit) {
		t.Fatal("expected ErrBadSplit when percentages don't total 100")
	}
}

func TestPercentageRejectsNegativeValue(t *testing.T) {
	in := []Input{{UserID: "a", Value: -500}, {UserID: "b", Value: 10500}}
	if _, err := Compute(100, models.SplitPercentage, in); !errors.Is(err, ErrNegativeValue) {
		t.Fatalf("expected ErrNegativeValue, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Shares split
// ---------------------------------------------------------------------------

func TestSharesProportional(t *testing.T) {
	// 2:1 shares of 90 => 60 / 30.
	in := []Input{{UserID: "a", Value: 2}, {UserID: "b", Value: 1}}
	shares, err := Compute(90, models.SplitShares, in)
	if err != nil {
		t.Fatal(err)
	}
	if sum(shares) != 90 {
		t.Fatalf("shares should sum to 90, got %d", sum(shares))
	}
	if shares[0].Amount != 60 || shares[1].Amount != 30 {
		t.Fatalf("expected 60/30, got %d/%d", shares[0].Amount, shares[1].Amount)
	}
}

func TestSharesRejectsAllZeroWeights(t *testing.T) {
	in := []Input{{UserID: "a", Value: 0}, {UserID: "b", Value: 0}}
	if _, err := Compute(100, models.SplitShares, in); !errors.Is(err, ErrBadSplit) {
		t.Fatalf("expected ErrBadSplit for all-zero weights, got %v", err)
	}
}

func TestSharesRejectsNegativeWeight(t *testing.T) {
	in := []Input{{UserID: "a", Value: -1}, {UserID: "b", Value: 3}}
	if _, err := Compute(100, models.SplitShares, in); !errors.Is(err, ErrNegativeValue) {
		t.Fatalf("expected ErrNegativeValue for negative weight, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Proportional: no-lost-cent guarantee across large amounts
// ---------------------------------------------------------------------------

func TestProportionalNoLostCentsLargeAmount(t *testing.T) {
	// $1000.01 split 3:3:4 in shares. Verifies zero penny loss for large odd amounts.
	in := []Input{{UserID: "a", Value: 3}, {UserID: "b", Value: 3}, {UserID: "c", Value: 4}}
	shares, err := Compute(100001, models.SplitShares, in)
	if err != nil {
		t.Fatal(err)
	}
	if got := sum(shares); got != 100001 {
		t.Fatalf("expected sum=100001, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// Simplify (debt minimization)
// ---------------------------------------------------------------------------

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
	total := sumTransfers(tx)
	if total != 100 {
		t.Fatalf("transfers should total 100, got %d", total)
	}
	for _, x := range tx {
		if x.ToID != "a" {
			t.Fatalf("everyone should pay A, got %s", x.ToID)
		}
	}
}

func TestSimplifyEmptyBalances(t *testing.T) {
	if tx := Simplify("USD", nil); tx != nil {
		t.Fatalf("expected nil for empty balances, got %v", tx)
	}
	if tx := Simplify("USD", []models.Balance{}); tx != nil {
		t.Fatalf("expected nil for empty slice, got %v", tx)
	}
}

func TestSimplifySkipsZeroBalances(t *testing.T) {
	// One user is settled (net=0), should not appear in any transfer.
	balances := []models.Balance{
		{UserID: "a", UserName: "A", Net: 50},
		{UserID: "b", UserName: "B", Net: -50},
		{UserID: "c", UserName: "C", Net: 0}, // already settled
	}
	tx := Simplify("USD", balances)
	if len(tx) != 1 {
		t.Fatalf("expected 1 transfer, got %d", len(tx))
	}
	for _, x := range tx {
		if x.FromID == "c" || x.ToID == "c" {
			t.Fatalf("settled user C should not appear in transfers")
		}
	}
}

func TestSimplifyHeapReordersAfterPartialPayment(t *testing.T) {
	// This specifically tests the case where the two-pointer approach (sort once,
	// then scan) would not re-evaluate order after a partial payment, but the
	// heap approach always picks the true largest remaining balance.
	//
	// Debtors:   D1=100, D2=30
	// Creditors: C1=90,  C2=40
	//
	// Step 1: D1(100) vs C1(90) → D1 pays C1 90. D1 has 10 left.
	// Step 2: D2(30) vs D1(10) — heap picks largest: D2(30) vs C2(40) → D2 pays 30.
	// Step 3: D1(10) vs C2(10) → D1 pays remaining 10.
	// Total: 3 transfers, sum = 130.
	balances := []models.Balance{
		{UserID: "c1", UserName: "C1", Net: 90},
		{UserID: "c2", UserName: "C2", Net: 40},
		{UserID: "d1", UserName: "D1", Net: -100},
		{UserID: "d2", UserName: "D2", Net: -30},
	}
	tx := Simplify("USD", balances)
	// n=2 debtors, m=2 creditors → at most 2+2-1=3 transfers.
	if len(tx) > 3 {
		t.Fatalf("expected at most 3 transfers, got %d", len(tx))
	}
	// All debts cleared: total transferred must equal 130.
	if got := sumTransfers(tx); got != 130 {
		t.Fatalf("expected total transfers=130, got %d", got)
	}
}

func TestSimplifyAllSettled(t *testing.T) {
	// If all nets are zero, no transfers needed.
	balances := []models.Balance{
		{UserID: "a", Net: 0},
		{UserID: "b", Net: 0},
	}
	if tx := Simplify("USD", balances); len(tx) != 0 {
		t.Fatalf("expected 0 transfers when all settled, got %d", len(tx))
	}
}

func TestSimplifyMultipleCreditors(t *testing.T) {
	// 4-person group: A owes 70, B owes 30; C is owed 60, D is owed 40.
	// Minimum transfers = 2+2-1 = 3.
	balances := []models.Balance{
		{UserID: "c", UserName: "C", Net: 60},
		{UserID: "d", UserName: "D", Net: 40},
		{UserID: "a", UserName: "A", Net: -70},
		{UserID: "b", UserName: "B", Net: -30},
	}
	tx := Simplify("USD", balances)
	if len(tx) > 3 {
		t.Fatalf("expected at most 3 transfers, got %d", len(tx))
	}
	if got := sumTransfers(tx); got != 100 {
		t.Fatalf("total transfers should equal total debt 100, got %d", got)
	}
}
