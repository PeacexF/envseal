package dotenv

// Delta is the difference between two environments, by variable name. Values
// are what it deliberately omits: a change must be reviewable without being
// readable.
type Delta struct {
	Added   []string `json:"added"`
	Changed []string `json:"changed"`
	Removed []string `json:"removed"`
}

// Compare reports how after differs from before. Added and changed names follow
// after's order; removed names follow before's.
func Compare(before, after *File) Delta {
	// Empty rather than nil, so JSON consumers always see a list.
	d := Delta{Added: []string{}, Changed: []string{}, Removed: []string{}}
	if before == nil || after == nil {
		return d
	}

	old, updated := before.Map(), after.Map()

	for _, key := range after.Keys() {
		previous, existed := old[key]
		switch {
		case !existed:
			d.Added = append(d.Added, key)
		case previous != updated[key]:
			d.Changed = append(d.Changed, key)
		}
	}
	for _, key := range before.Keys() {
		if _, still := updated[key]; !still {
			d.Removed = append(d.Removed, key)
		}
	}
	return d
}

func (d Delta) Len() int { return len(d.Added) + len(d.Changed) + len(d.Removed) }

func (d Delta) Empty() bool { return d.Len() == 0 }
