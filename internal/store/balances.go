package store

import (
	"sort"

	"splitwise-quic/internal/models"
	"splitwise-quic/internal/splits"
)

// Balances computes each member's net position per currency for a group.
// Convention: net > 0 => the user is owed money; net < 0 => the user owes.
// Returns a map keyed by currency code.
func (s *Store) Balances(groupID string) (map[string][]models.Balance, error) {
	names, err := s.memberNames(groupID)
	if err != nil {
		return nil, err
	}
	// currency -> userID -> net
	nets := map[string]map[string]int64{}
	ensure := func(cur string) map[string]int64 {
		if nets[cur] == nil {
			nets[cur] = map[string]int64{}
		}
		return nets[cur]
	}

	expenses, err := s.Expenses(groupID)
	if err != nil {
		return nil, err
	}
	for _, e := range expenses {
		m := ensure(e.Currency)
		m[e.PaidBy] += e.Amount // they fronted the cash
		for _, sh := range e.Shares {
			m[sh.UserID] -= sh.Amount // they consumed value
		}
	}

	settlements, err := s.Settlements(groupID)
	if err != nil {
		return nil, err
	}
	for _, st := range settlements {
		m := ensure(st.Currency)
		m[st.FromUser] += st.Amount // debtor paid down their balance
		m[st.ToUser] -= st.Amount   // creditor received funds
	}

	out := map[string][]models.Balance{}
	for cur, m := range nets {
		var list []models.Balance
		for uid, net := range m {
			if net == 0 {
				continue
			}
			list = append(list, models.Balance{
				UserID: uid, UserName: names[uid], Currency: cur, Net: net,
			})
		}
		sort.SliceStable(list, func(i, j int) bool { return list[i].Net > list[j].Net })
		out[cur] = list
	}
	return out, nil
}

// SimplifiedTransfers returns the minimized "who pays whom" plan per currency.
func (s *Store) SimplifiedTransfers(groupID string) (map[string][]models.Transfer, error) {
	balances, err := s.Balances(groupID)
	if err != nil {
		return nil, err
	}
	out := map[string][]models.Transfer{}
	for cur, list := range balances {
		out[cur] = splits.Simplify(cur, list)
	}
	return out, nil
}

func (s *Store) memberNames(groupID string) (map[string]string, error) {
	members, err := s.GroupMembers(groupID)
	if err != nil {
		return nil, err
	}
	names := make(map[string]string, len(members))
	for _, m := range members {
		names[m.ID] = m.Name
	}
	return names, nil
}
