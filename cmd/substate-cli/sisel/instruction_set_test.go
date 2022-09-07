package sisel

import (
	"testing"
)

func TestInstructionSetIsValidMapKey(t *testing.T) {
	// This test already passes if it compiles
	var _ map[InstructionSet]int
}

func TestEmptyStringEncodesEmptySet(t *testing.T) {
	set, err := decodeInstructionSet("")
	if err != nil {
		t.Errorf("Failed decoding empty string: %v", err)
		return
	}
	if len(set) != 0 {
		t.Errorf("Decoding empty string yielded non-empty set: %v", set)
		return
	}
}

func TestEncodingAndDecodingSets(t *testing.T) {
	inputs := []struct {
		set map[SuperInstructionId]int
	}{
		{set: map[SuperInstructionId]int{}},
		{set: map[SuperInstructionId]int{
			SuperInstructionId(0): 0,
		}},
		{set: map[SuperInstructionId]int{
			SuperInstructionId(0): 0,
			SuperInstructionId(1): 0,
		}},
		{set: map[SuperInstructionId]int{
			SuperInstructionId(0): 0,
			SuperInstructionId(2): 0,
			SuperInstructionId(5): 0,
		}},
	}

	for _, input := range inputs {
		encoded := encodeInstructionSet(input.set)

		if l := len(encoded); l != len(input.set)*4 {
			t.Errorf("Encoded set has invalid length, expected %d, got %d", len(input.set)*4, len(encoded))
			continue
		}

		restored, err := decodeInstructionSet(encoded)
		if err != nil {
			t.Errorf("Error decoding input set %v: %v", input.set, err)
			continue
		}

		if len(input.set) != len(restored) {
			t.Errorf("Restored set not of equal length %v vs. %v", len(input.set), len(restored))
			continue
		}

		for k := range input.set {
			if _, present := restored[k]; !present {
				t.Errorf("Missing entry %v", k)
			}
		}
	}
}

func makeSetFromInt(ids []int) InstructionSet {
	list := make([]SuperInstructionId, len(ids))
	for i, val := range ids {
		list[i] = SuperInstructionId(val)
	}
	return MakeSet(list)
}

func TestMakeSet(t *testing.T) {
	inputs := []struct {
		elements []int
		text     string
	}{
		{
			elements: []int{},
			text:     "{}",
		},
		{
			elements: []int{1},
			text:     "{1}",
		},
		{
			elements: []int{1, 2},
			text:     "{1 2}",
		},
		{
			elements: []int{1, 2, 3},
			text:     "{1 2 3}",
		},
		{
			elements: []int{2, 1},
			text:     "{1 2}",
		},
		{
			elements: []int{2, 1, 2},
			text:     "{1 2}",
		},
		{
			elements: []int{1, 2, 1, 2},
			text:     "{1 2}",
		},
	}

	for _, input := range inputs {
		set := makeSetFromInt(input.elements)
		if got := set.String(); got != input.text {
			t.Errorf("Creation of set failed: wanted %v, got %v", input.text, got)
		}
	}
}

func TestSetContains(t *testing.T) {
	inputs := []struct {
		a []int
		b int
		r bool
	}{
		{a: []int{}, b: 1, r: false},
		{a: []int{1}, b: 1, r: true},
		{a: []int{1}, b: 2, r: false},
		{a: []int{1, 2}, b: 2, r: true},
	}

	for _, input := range inputs {
		a := makeSetFromInt(input.a)
		b := SuperInstructionId(input.b)
		r := input.r
		if got := a.Contains(b); got != r {
			t.Errorf("%v.Contains(%v) failed: wanted %v, got %v", a, b, r, got)
		}
	}
}

func TestSetContainsAll(t *testing.T) {
	inputs := []struct {
		a []int
		b []int
		r bool
	}{
		{a: []int{}, b: []int{}, r: true},
		{a: []int{1}, b: []int{}, r: true},
		{a: []int{1, 2}, b: []int{}, r: true},
		{a: []int{}, b: []int{1}, r: false},
		{a: []int{}, b: []int{1, 2}, r: false},
		{a: []int{1}, b: []int{1}, r: true},
		{a: []int{1, 2}, b: []int{1}, r: true},
		{a: []int{1, 2, 3}, b: []int{1, 3}, r: true},
		{a: []int{1, 2, 3}, b: []int{1, 4}, r: false},
	}

	for _, input := range inputs {
		a := makeSetFromInt(input.a)
		b := makeSetFromInt(input.b)
		r := input.r
		if got := a.ContainsAll(b); got != r {
			t.Errorf("%v.Contains(%v) failed: wanted %v, got %v", a, b, r, got)
		}
	}
}

func TestSetAdd(t *testing.T) {
	inputs := []struct {
		a []int
		b int
		r []int
	}{
		{a: []int{}, b: 1, r: []int{1}},
		{a: []int{1}, b: 1, r: []int{1}},
		{a: []int{1}, b: 2, r: []int{1, 2}},
		{a: []int{1, 2}, b: 2, r: []int{1, 2}},
	}

	for _, input := range inputs {
		a := makeSetFromInt(input.a)
		b := makeSetFromInt([]int{input.b})
		r := makeSetFromInt(input.r)
		if got := Union(a, b); got != r {
			t.Errorf("Union of %v and %v failed: wanted %v, got %v", a, b, r, got)
		}
	}
}

func TestSetRemove(t *testing.T) {
	inputs := []struct {
		a []int
		b int
		r []int
	}{
		{a: []int{}, b: 1, r: []int{}},
		{a: []int{1}, b: 1, r: []int{}},
		{a: []int{1, 2}, b: 1, r: []int{2}},
		{a: []int{1, 2}, b: 2, r: []int{1}},
		{a: []int{1, 2, 3}, b: 0, r: []int{1, 2, 3}},
		{a: []int{1, 2, 3}, b: 1, r: []int{2, 3}},
		{a: []int{1, 2, 3}, b: 2, r: []int{1, 3}},
		{a: []int{1, 2, 3}, b: 3, r: []int{1, 2}},
		{a: []int{1, 2, 3}, b: 4, r: []int{1, 2, 3}},
	}

	for _, input := range inputs {
		a := makeSetFromInt(input.a)
		b := SuperInstructionId(input.b)
		r := makeSetFromInt(input.r)
		if got := a.Remove(b); got != r {
			t.Errorf("Removing %v from %v failed: wanted %v, got %v", b, a, r, got)
		}
	}
}

func TestSetUnion(t *testing.T) {
	inputs := []struct {
		a []int
		b []int
		r []int
	}{
		{a: []int{}, b: []int{}, r: []int{}},
		{a: []int{1}, b: []int{}, r: []int{1}},
		{a: []int{}, b: []int{1}, r: []int{1}},
		{a: []int{1}, b: []int{1}, r: []int{1}},
		{a: []int{1}, b: []int{2}, r: []int{1, 2}},
		{a: []int{1, 3}, b: []int{2}, r: []int{1, 2, 3}},
		{a: []int{1, 3}, b: []int{2, 4}, r: []int{1, 2, 3, 4}},
		{a: []int{1, 2, 3}, b: []int{2, 4}, r: []int{1, 2, 3, 4}},
	}

	for _, input := range inputs {
		a := makeSetFromInt(input.a)
		b := makeSetFromInt(input.b)
		r := makeSetFromInt(input.r)
		if got := Union(a, b); got != r {
			t.Errorf("Union of %v and %v failed: wanted %v, got %v", a, b, r, got)
		}
	}
}

func TestSetIntersection(t *testing.T) {
	inputs := []struct {
		a []int
		b []int
		r []int
	}{
		{a: []int{}, b: []int{}, r: []int{}},
		{a: []int{1}, b: []int{}, r: []int{}},
		{a: []int{}, b: []int{1}, r: []int{}},
		{a: []int{1}, b: []int{1}, r: []int{1}},
		{a: []int{1}, b: []int{2}, r: []int{}},
		{a: []int{1, 2}, b: []int{3, 4}, r: []int{}},
		{a: []int{1, 3}, b: []int{2}, r: []int{}},
		{a: []int{1, 3}, b: []int{2, 4}, r: []int{}},
		{a: []int{1, 2, 3}, b: []int{2, 4}, r: []int{2}},
		{a: []int{1, 2, 3, 4}, b: []int{2, 3}, r: []int{2, 3}},
		{a: []int{3, 4}, b: []int{1, 2, 3}, r: []int{3}},
	}

	for _, input := range inputs {
		a := makeSetFromInt(input.a)
		b := makeSetFromInt(input.b)
		r := makeSetFromInt(input.r)
		if got := Intersect(a, b); got != r {
			t.Errorf("Intersection of %v and %v failed: wanted %v, got %v", a, b, r, got)
		}
	}
}

func TestSetDifference(t *testing.T) {
	inputs := []struct {
		a []int
		b []int
		r []int
	}{
		{a: []int{}, b: []int{}, r: []int{}},
		{a: []int{1}, b: []int{}, r: []int{1}},
		{a: []int{}, b: []int{1}, r: []int{}},
		{a: []int{1}, b: []int{1}, r: []int{}},
		{a: []int{1}, b: []int{2}, r: []int{1}},
		{a: []int{1, 2}, b: []int{3, 4}, r: []int{1, 2}},
		{a: []int{1, 3}, b: []int{2}, r: []int{1, 3}},
		{a: []int{1, 3}, b: []int{2, 4}, r: []int{1, 3}},
		{a: []int{1, 2, 3}, b: []int{2, 4}, r: []int{1, 3}},
		{a: []int{1, 2, 3, 4}, b: []int{2, 3}, r: []int{1, 4}},
		{a: []int{3, 4}, b: []int{1, 2, 3}, r: []int{4}},
	}

	for _, input := range inputs {
		a := makeSetFromInt(input.a)
		b := makeSetFromInt(input.b)
		r := makeSetFromInt(input.r)
		if got := Difference(a, b); got != r {
			t.Errorf("Difference of %v and %v failed: wanted %v, got %v", a, b, r, got)
		}
	}
}

func TestGetSubsets(t *testing.T) {
	inputs := []struct {
		a []int
	}{
		{a: []int{}},
		{a: []int{1}},
		{a: []int{1, 2}},
		{a: []int{1, 2, 4}},
	}

	for _, input := range inputs {
		a := makeSetFromInt(input.a)
		subsets := a.GetSubsets()

		// Check that there is the right number of sets.
		want := 1 << a.Size()
		if got := len(subsets); got != want {
			t.Errorf("Number of subsets of %v is not as expected: wanted %v, got %v", a, want, got)
		}

		// Check that they are all different.
		set := map[InstructionSet]int{}
		for _, s := range subsets {
			set[s] = 0
		}
		if got := len(set); got != want {
			t.Errorf("Not all subsets are distinct: %v", set)
		}

		// Check that the union of the subsets is the set itself
		all := InstructionSet{}
		for _, s := range subsets {
			all = Union(all, s)
		}
		if got := all; got != a {
			t.Errorf("Union of subsets is not the full set: Union(%v)=%v != %v", set, all, a)
		}
	}
}
