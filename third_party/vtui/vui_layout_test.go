package vtui

import "testing"

func sizesEqual(t *testing.T, got, want []int) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("length mismatch: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("index %d: got %d, want %d (full: got=%v want=%v)", i, got[i], want[i], got, want)
		}
	}
}

func TestDistribute1D_EmptyItems(t *testing.T) {
	sizes, positions := Distribute1D(100, nil, 1, 2, 2)
	if len(sizes) != 0 || len(positions) != 0 {
		t.Fatalf("expected empty slices for zero items, got sizes=%v positions=%v", sizes, positions)
	}
}

func TestDistribute1D_NegativeUsableClampsToZeroThenFloorsAtMin(t *testing.T) {
	// Margins alone already exceed length, so usable space clamps to 0 and
	// the item can only be reduced down to its Min, never below.
	items := []SizeSpec{{Hint: 5, Min: 3, Policy: PolicyPreferred, Stretch: 1}}
	sizes, _ := Distribute1D(1, items, 0, 5, 5)
	sizesEqual(t, sizes, []int{3})
}

func TestDistribute1D_SurplusFallsBackToPreferredWhenNoExpanding(t *testing.T) {
	// With no PolicyExpanding items, surplus must fall back to
	// preferred/minimum candidates, split evenly by (implicit) stretch 1.
	items := []SizeSpec{
		{Hint: 2, Min: 1, Policy: PolicyPreferred, Stretch: 0},
		{Hint: 2, Min: 1, Policy: PolicyMinimum, Stretch: 0},
	}
	sizes, _ := Distribute1D(10, items, 0, 0, 0)
	sizesEqual(t, sizes, []int{5, 5})
}

func TestDistribute1D_SurplusMaxClampReclaimsAcrossIterations(t *testing.T) {
	// A lands on its Max in the first pass; the surplus it can't absorb must
	// be reclaimed and handed to B on a subsequent iteration.
	items := []SizeSpec{
		{Hint: 2, Min: 0, Max: 5, Policy: PolicyExpanding, Stretch: 1},
		{Hint: 2, Min: 0, Max: 0, Policy: PolicyExpanding, Stretch: 1},
	}
	sizes, _ := Distribute1D(20, items, 0, 0, 0)
	sizesEqual(t, sizes, []int{5, 15})
}

func TestDistribute1D_SurplusRemainderGoesToEarlyCandidates(t *testing.T) {
	// 10 does not split evenly across 3 equal-weight candidates; the leftover
	// remainder cell must go to the earliest candidates in order.
	items := []SizeSpec{
		{Hint: 0, Min: 0, Policy: PolicyExpanding, Stretch: 1},
		{Hint: 0, Min: 0, Policy: PolicyExpanding, Stretch: 1},
		{Hint: 0, Min: 0, Policy: PolicyExpanding, Stretch: 1},
	}
	sizes, _ := Distribute1D(10, items, 0, 0, 0)
	sizesEqual(t, sizes, []int{4, 3, 3})
}

func TestDistribute1D_FixedPolicyNeverReceivesSurplus(t *testing.T) {
	items := []SizeSpec{
		{Hint: 5, Min: 5, Max: 5, Policy: PolicyFixed, Stretch: 1},
		{Hint: 0, Min: 0, Policy: PolicyExpanding, Stretch: 1},
	}
	sizes, _ := Distribute1D(15, items, 0, 0, 0)
	sizesEqual(t, sizes, []int{5, 10})
}

func TestDistribute1D_DeficitReclaimsProportionallyAboveMin(t *testing.T) {
	// usable is smaller than the hinted sum; the deficit must be pulled from
	// the item that has slack above its Min, leaving the other untouched.
	items := []SizeSpec{
		{Hint: 5, Min: 5, Policy: PolicyPreferred, Stretch: 1}, // no slack: already at Min
		{Hint: 5, Min: 0, Policy: PolicyPreferred, Stretch: 1}, // all the slack
	}
	sizes, _ := Distribute1D(8, items, 0, 0, 0)
	sizesEqual(t, sizes, []int{5, 3})
}

func TestDistribute1D_DeficitCannotShrinkBelowMinimum(t *testing.T) {
	// Both items already sit exactly at their Min, so the deficit-reclaim
	// loop must find no candidates and stop, even though the requested
	// length still overflows the available space.
	items := []SizeSpec{
		{Hint: 5, Min: 5, Policy: PolicyPreferred, Stretch: 1},
		{Hint: 5, Min: 5, Policy: PolicyPreferred, Stretch: 1},
	}
	sizes, _ := Distribute1D(6, items, 0, 0, 0)
	sizesEqual(t, sizes, []int{5, 5})
}

func TestDistribute1D_PositionsRespectMarginAndSpacing(t *testing.T) {
	items := []SizeSpec{
		{Hint: 3, Min: 3, Max: 3, Policy: PolicyFixed, Stretch: 1},
		{Hint: 4, Min: 4, Max: 4, Policy: PolicyFixed, Stretch: 1},
	}
	sizes, positions := Distribute1D(20, items, 2, 1, 1)
	sizesEqual(t, sizes, []int{3, 4})
	if positions[0] != 1 {
		t.Errorf("first position = %d, want marginBefore (1)", positions[0])
	}
	if positions[1] != positions[0]+sizes[0]+2 {
		t.Errorf("second position = %d, want %d", positions[1], positions[0]+sizes[0]+2)
	}
}
