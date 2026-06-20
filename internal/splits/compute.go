// Package splits contains the pure money-math: dividing expenses and
// minimizing the number of transfers needed to settle a group.
//
// All arithmetic is done in integer minor units (cents). No floating-point
// is used; rounding remainders are distributed deterministically.
package splits

import (
	"errors"
	"math"

	"splitwise-quic/internal/models"
)

// Sentinel errors — use errors.Is() to distinguish them.
var (
	// ErrEmptyInputs is returned when no participants are provided.
	ErrEmptyInputs = errors.New("splits: no participants provided")

	// ErrNonPositiveTotal is returned when the expense total is zero or negative.
	ErrNonPositiveTotal = errors.New("splits: total must be greater than zero")

	// ErrDuplicateUser is returned when the same UserID appears more than once.
	ErrDuplicateUser = errors.New("splits: duplicate user ID in split inputs")

	// ErrNegativeValue is returned when a per-user split value is negative.
	ErrNegativeValue = errors.New("splits: split value must not be negative")

	// ErrOverflow is returned when intermediate arithmetic would exceed int64.
	ErrOverflow = errors.New("splits: arithmetic overflow — reduce amount or share weight")

	// ErrInvalidSplitType is returned for an unrecognised SplitType string.
	ErrInvalidSplitType = errors.New("splits: unknown split type")

	// ErrBadSplit is returned when values don't reconcile with the total
	// (exact amounts don't sum to total, or percentages don't sum to 100%).
	ErrBadSplit = errors.New("splits: values do not reconcile with the total amount")
)

// Input describes one participant's parameter for a split.
//   - equal:      Value ignored (everyone splits evenly)
//   - exact:      Value = exact minor units this user owes (must be ≥ 0)
//   - percentage: Value = basis points (hundredths of a percent, e.g. 33.33% = 3333)
//   - shares:     Value = integer weight (must be ≥ 0)
type Input struct {
	UserID string
	Value  int64
}

// Compute turns a total amount + split inputs into concrete per-user shares.
//
// Guarantees:
//   - Shares always sum exactly to total (no lost pennies).
//   - Rounding remainder is distributed one cent at a time to the participants
//     with the largest fractional parts (standard "largest-remainder" method).
//
// Time: O(n) for equal/exact; O(n × r) for proportional modes where r < n is the
// rounding remainder (typically 0–2 cents). Space: O(n).
func Compute(total int64, st models.SplitType, inputs []Input) ([]models.Share, error) {
	if total <= 0 {
		return nil, ErrNonPositiveTotal
	}
	if len(inputs) == 0 {
		return nil, ErrEmptyInputs
	}

	// O(n) duplicate-user check.
	seen := make(map[string]struct{}, len(inputs))
	for _, p := range inputs {
		if _, dup := seen[p.UserID]; dup {
			return nil, ErrDuplicateUser
		}
		seen[p.UserID] = struct{}{}
	}

	switch st {
	case models.SplitEqual:
		return equal(total, inputs), nil
	case models.SplitExact:
		return exact(total, inputs)
	case models.SplitPercentage:
		return percentage(total, inputs)
	case models.SplitShares:
		return shares(total, inputs)
	default:
		return nil, ErrInvalidSplitType
	}
}

// equal divides total evenly. Any remainder cents go to the first participants
// one by one — deterministic and independent of participant ordering.
// Time: O(n). Space: O(n).
func equal(total int64, in []Input) []models.Share {
	n := int64(len(in))
	base := total / n
	rem := total - base*n
	out := make([]models.Share, len(in))
	for i, p := range in {
		amt := base
		if int64(i) < rem {
			amt++ // distribute leftover cents to the first `rem` participants
		}
		out[i] = models.Share{UserID: p.UserID, Amount: amt}
	}
	return out
}

// exact validates that per-user amounts reconcile to total.
// Time: O(n). Space: O(n).
func exact(total int64, in []Input) ([]models.Share, error) {
	var sum int64
	out := make([]models.Share, len(in))
	for i, p := range in {
		if p.Value < 0 {
			return nil, ErrNegativeValue
		}
		out[i] = models.Share{UserID: p.UserID, Amount: p.Value}
		sum += p.Value
	}
	if sum != total {
		return nil, ErrBadSplit
	}
	return out, nil
}

// percentage splits by basis points (100.00% = 10000 bps).
// All basis-point values must be non-negative and must sum to exactly 10000.
// Time: O(n × r) where r = rounding remainder < n. Space: O(n).
func percentage(total int64, in []Input) ([]models.Share, error) {
	var bps int64
	for _, p := range in {
		if p.Value < 0 {
			return nil, ErrNegativeValue
		}
		bps += p.Value
	}
	if bps != 10000 {
		return nil, ErrBadSplit
	}
	return proportional(total, in, 10000)
}

// shares splits by weighted ratios. All weights must be non-negative and
// at least one must be positive. Time: O(n × r). Space: O(n).
func shares(total int64, in []Input) ([]models.Share, error) {
	var sum int64
	for _, p := range in {
		if p.Value < 0 {
			return nil, ErrNegativeValue
		}
		sum += p.Value
	}
	if sum == 0 {
		return nil, ErrBadSplit
	}
	return proportional(total, in, sum)
}

// proportional allocates total in the ratio Value/denom per participant, then
// distributes the rounding remainder via the largest-remainder method.
//
// Overflow guard: detects when total×Value would exceed int64 before dividing.
//
// Remainder distribution uses a linear partial-maximum scan instead of a full
// sort: since the remainder is always < n (and typically 0–2 cents), this is
// O(n × r) — O(n) in practice — rather than O(n log n).
func proportional(total int64, in []Input, denom int64) ([]models.Share, error) {
	type frac struct {
		idx       int
		remainder int64
		consumed  bool
	}

	out := make([]models.Share, len(in))
	fracs := make([]frac, len(in))
	var assigned int64

	for i, p := range in {
		// Overflow guard: total * p.Value overflows int64 when p.Value > MaxInt64/total.
		if p.Value > 0 && total > math.MaxInt64/p.Value {
			return nil, ErrOverflow
		}
		num := total * p.Value
		amt := num / denom
		out[i] = models.Share{UserID: p.UserID, Amount: amt}
		fracs[i] = frac{idx: i, remainder: num % denom}
		assigned += amt
	}

	// Distribute leftover cents to the participants with the largest fractional
	// remainders. O(n × left) where left = total - assigned < n.
	// We do a linear max-scan for each penny rather than sorting the whole array,
	// since left is typically 0–2 and n is typically 2–20.
	left := total - assigned
	for k := int64(0); k < left; k++ {
		best := -1
		for j := range fracs {
			if fracs[j].consumed {
				continue
			}
			if best == -1 || fracs[j].remainder > fracs[best].remainder {
				best = j
			}
		}
		if best >= 0 {
			out[fracs[best].idx].Amount++
			fracs[best].consumed = true
		}
	}
	return out, nil
}
