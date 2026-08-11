package dotenv_test

import (
	"slices"
	"testing"

	"github.com/PeacexF/envseal/internal/dotenv"
)

func compare(t *testing.T, before, after string) dotenv.Delta {
	t.Helper()
	return dotenv.Compare(parse(t, before), parse(t, after))
}

func TestCompare(t *testing.T) {
	tests := []struct {
		name                    string
		before, after           string
		added, changed, removed []string
	}{
		{
			name:   "no changes",
			before: "A=1\nB=2\n", after: "A=1\nB=2\n",
		},
		{
			name:   "added",
			before: "A=1\n", after: "A=1\nB=2\n",
			added: []string{"B"},
		},
		{
			name:   "changed",
			before: "A=1\n", after: "A=2\n",
			changed: []string{"A"},
		},
		{
			name:   "removed",
			before: "A=1\nB=2\n", after: "A=1\n",
			removed: []string{"B"},
		},
		{
			name:   "all three",
			before: "KEEP=1\nCHANGE=old\nGONE=1\n", after: "KEEP=1\nCHANGE=new\nNEW=1\n",
			added: []string{"NEW"}, changed: []string{"CHANGE"}, removed: []string{"GONE"},
		},
		{
			name:   "reordering is not a change",
			before: "A=1\nB=2\n", after: "B=2\nA=1\n",
		},
		{
			name:   "comments are not variables",
			before: "A=1\n", after: "# a comment\nA=1\n",
		},
		{
			name:   "quoting style is not a change",
			before: "A=value\n", after: "A=\"value\"\n",
		},
		{
			name:   "duplicate keys compare by effective value",
			before: "A=1\nA=2\n", after: "A=2\n",
		},
		{
			name:   "empty before",
			before: "", after: "A=1\n",
			added: []string{"A"},
		},
		{
			name:   "empty after",
			before: "A=1\n", after: "",
			removed: []string{"A"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compare(t, tt.before, tt.after)

			if !equal(got.Added, tt.added) {
				t.Errorf("Added = %v, want %v", got.Added, tt.added)
			}
			if !equal(got.Changed, tt.changed) {
				t.Errorf("Changed = %v, want %v", got.Changed, tt.changed)
			}
			if !equal(got.Removed, tt.removed) {
				t.Errorf("Removed = %v, want %v", got.Removed, tt.removed)
			}

			wantLen := len(tt.added) + len(tt.changed) + len(tt.removed)
			if got.Len() != wantLen {
				t.Errorf("Len() = %d, want %d", got.Len(), wantLen)
			}
			if got.Empty() != (wantLen == 0) {
				t.Errorf("Empty() = %v, want %v", got.Empty(), wantLen == 0)
			}
		})
	}
}

func TestCompareFollowsSourceOrder(t *testing.T) {
	got := compare(t, "OLD=1\n", "ZED=1\nALPHA=2\nMID=3\n")

	if want := []string{"ZED", "ALPHA", "MID"}; !slices.Equal(got.Added, want) {
		t.Errorf("Added = %v, want source order %v", got.Added, want)
	}
}

// JSON consumers should always see a list, never null.
func TestCompareReturnsEmptyLists(t *testing.T) {
	got := compare(t, "A=1\n", "A=1\n")

	if got.Added == nil || got.Changed == nil || got.Removed == nil {
		t.Errorf("Compare() = %+v, want empty lists rather than nil", got)
	}
}

func TestCompareNil(t *testing.T) {
	if got := dotenv.Compare(nil, nil); !got.Empty() {
		t.Errorf("Compare(nil, nil) = %+v, want empty", got)
	}
}

func equal(got, want []string) bool {
	if len(got) == 0 && len(want) == 0 {
		return true
	}
	return slices.Equal(got, want)
}
