// Package splits contains the pure money-math: dividing expenses and
// minimizing the number of transfers needed to settle a group.
package splits

import (
	"errors"
	"sort"

	"splitwise-quic/internal/models"
)

// Input describes one participant's parameter for a split.
//   - equal:      Value ignored (everyone splits evenly)
//   - exact:      Value = exact minor units this user owes
//   - percentage: Value = percentage (0-100, may be fractional *100, see below)
//   - shares:     Value = integer weight
//
// For percentage we accept basis points (hundredths of a percent) so that
// 33.33% is expressed as 3333 and totals must sum to 10000.
type Input struct {
	UserID string
	Value  int64
}

// ErrBadSplit indicates the inputs don't add up (e.g. exact != total).
var ErrBadSplit = errors.New("split values do not reconcile with the total amount")

// Compute turns a total amount + split inputs into concrete per-user shares.
// It guarantees the shares sum *exactly* to total by spreading any rounding
// remainder one cent at a time across the largest participants — the same
// trick real ledgers use to avoid the dreaded missing penny.
func Compute(total int64, st models.SplitType, inputs []Input) ([]models.Share, error) {
	if len(inputs) == 0 {
		return nil, ErrBadSplit
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
		return nil, ErrBadSplit
	}
}

func equal(total int64, in []Input) []models.Share {
	n := int64(len(in))
	base := total / n
	rem := total - base*n
	out := make([]models.Share, len(in))
	for i, p := range in {
		amt := base
		if int64(i) < rem { // hand out leftover cents to the first few
			amt++
		}
		out[i] = models.Share{UserID: p.UserID, Amount: amt}
	}
	return out
}

func exact(total int64, in []Input) ([]models.Share, error) {
	var sum int64
	out := make([]models.Share, len(in))
	for i, p := range in {
		out[i] = models.Share{UserID: p.UserID, Amount: p.Value}
		sum += p.Value
	}
	if sum != total {
		return nil, ErrBadSplit
	}
	return out, nil
}

func percentage(total int64, in []Input) ([]models.Share, error) {
	var bps int64
	for _, p := range in {
		bps += p.Value
	}
	if bps != 10000 { // must total 100.00%
		return nil, ErrBadSplit
	}
	return proportional(total, in, 10000), nil
}

func shares(total int64, in []Input) ([]models.Share, error) {
	var sum int64
	for _, p := range in {
		if p.Value < 0 {
			return nil, ErrBadSplit
		}
		sum += p.Value
	}
	if sum == 0 {
		return nil, ErrBadSplit
	}
	return proportional(total, in, sum), nil
}

// proportional divides total in the ratio Value/denominator, then distributes
// the rounding remainder to the participants with the largest fractional parts.
func proportional(total int64, in []Input, denom int64) []models.Share {
	out := make([]models.Share, len(in))
	type frac struct {
		idx       int
		remainder int64
	}
	var assigned int64
	fracs := make([]frac, len(in))
	for i, p := range in {
		num := total * p.Value
		amt := num / denom
		out[i] = models.Share{UserID: p.UserID, Amount: amt}
		fracs[i] = frac{idx: i, remainder: num % denom}
		assigned += amt
	}
	left := total - assigned
	// Give the leftover cents to the biggest fractional remainders first.
	sort.SliceStable(fracs, func(a, b int) bool {
		return fracs[a].remainder > fracs[b].remainder
	})
	for i := int64(0); i < left; i++ {
		out[fracs[i].idx].Amount++
	}
	return out
}
