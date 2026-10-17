// Package reporting defines the shared calendar interval for aggregate reads.
package reporting

import "time"

type Period struct {
	From string `json:"from"`
	To   string `json:"to"`
}

func (p Period) Valid() bool {
	for _, raw := range []string{p.From, p.To} {
		date, err := time.Parse(time.DateOnly, raw)
		if err != nil || len(raw) != 10 || date.Year() < 1 || date.Format(time.DateOnly) != raw {
			return false
		}
	}
	return p.From <= p.To
}
