package models

import (
	"fmt"
	"math"
	"testing"
)

func TestEqualityValueCollectionsIdentity(t *testing.T) {
	ptr := &Bad{Dynamic: []int{1}}
	channel := make(chan int)
	a := Record{Scalar: 1, Array: Fixed{2, 3}, Aggregate: Value{Number: 4}, Pointer: ptr, Channel: channel, private: 5}
	b := a
	b.Slice = Values{}
	b.Map = Lookup{}
	if !a.Equal(b) {
		t.Fatal("nil/empty collection equality changed")
	}
	a.Slice = Values{{Number: 6}}
	b.Slice = Values{{Number: 6}}
	a.Map = Lookup{"a": {Number: 7}}
	b.Map = Lookup{"a": {Number: 7}}
	if !a.Equal(b) {
		t.Fatal("named collections/values did not compare")
	}
	b.private = 8
	if a.Equal(b) {
		t.Fatal("private field excluded")
	}
	b.private = a.private
	b.Pointer = &Bad{Dynamic: []int{1}}
	if a.Equal(b) {
		t.Fatal("pointer compared structurally")
	}
	b.Pointer = ptr
	b.Channel = make(chan int)
	if a.Equal(b) {
		t.Fatal("channel identity lost")
	}
	b.Channel = channel
	b.Slice[0].Number = 9
	if a.Equal(b) {
		t.Fatal("slice difference ignored")
	}
	b.Slice[0].Number = 6
	b.Map["a"] = Value{Number: 10}
	if a.Equal(b) {
		t.Fatal("map difference ignored")
	}
	if (Float{Value: math.NaN()}).Equal(Float{Value: math.NaN()}) {
		t.Fatal("NaN semantics changed")
	}
	if !(Empty{}).Equal(Empty{}) || (Empty{}).String() != "Empty{}" || !(Skipped{}).Equal(Skipped{}) || (Skipped{}).String() != "Skipped{}" {
		t.Fatal("empty output wrong")
	}
}

func TestEqualSourceOrderAndShortCircuiting(t *testing.T) {
	// Source order compares A first here, so a slice-valued Z is never compared.
	left := ShortCircuit[any]{A: 1, Z: []int{1}}
	right := ShortCircuit[any]{A: 2, Z: []int{1}}
	if left.Equal(right) {
		t.Fatal("unequal first field did not short-circuit")
	}
	if (FilteredPair[any]{Ignored: []int{1}, A: 1, Z: []int{1}}).Equal(FilteredPair[any]{Ignored: []int{1}, A: 2, Z: []int{1}}) {
		t.Fatal("skip-equal changed the remaining source-order comparisons")
	}
	if !(Pair[int]{Z: 1, A: 2}).Equal(Pair[int]{Z: 1, A: 2}) || (Pair[int]{Z: 1, A: 2}).Equal(Pair[int]{Z: 1, A: 3}) {
		t.Fatal("comparable source-order equality changed")
	}
	// Previously order=name would compare A first and return false. Z now runs
	// first, and its slice-valued interface comparison must panic despite A.
	defer func() {
		if recover() == nil {
			t.Fatal("source-first Z comparison did not panic")
		}
	}()
	(Pair[any]{Z: []int{1}, A: 1}).Equal(Pair[any]{Z: []int{1}, A: 2})
}

func TestStringOrderingFilteringAndMixedOutputs(t *testing.T) {
	d := Display{Z: 2, a: "private", embed: embed{X: 3}, Hidden: 4}
	expected := fmt.Sprintf("Display{Z:%#v, a:%#v, embed:%#v, Printer:%#v}", d.Z, d.a, d.embed, d.Printer)
	if d.String() != expected {
		t.Fatalf("string format: %s != %s", d.String(), expected)
	}
	m := NewMixedBuilder().Value(7).Note("first").Build()
	if m.Hidden() != 0 || m.String() != `Mixed{hidden:0, Value:7, Note:"first"}` || !m.Equal(m) {
		t.Fatal("outputs altered canonical independent field sets")
	}
	n := m
	n.hidden = 9
	if m.Equal(n) {
		t.Fatal("skip-builder also skipped equality")
	}
}

func TestGenericComparableAndCollectionConstraints(t *testing.T) {
	if !(Box[int]{Value: 1}).Equal(Box[int]{Value: 1}) || (Box[int]{Value: 1}).String() != "Box{Value:1}" {
		t.Fatal("generic scalar output wrong")
	}
	if !(GenericSlice[int, Ints]{}).Equal(GenericSlice[int, Ints]{Values: Ints{}}) {
		t.Fatal("generic slice nil/empty wrong")
	}
	if !(GenericUnion[Ints]{Values: Ints{1}}).Equal(GenericUnion[Ints]{Values: Ints{1}}) {
		t.Fatal("uniform union shape not supported")
	}
	if !(GenericMap[string, int, map[string]int]{}).Equal(GenericMap[string, int, map[string]int]{Values: map[string]int{}}) {
		t.Fatal("generic map nil/empty wrong")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("generic interface comparison unexpectedly advertised as panic-free")
		}
	}()
	(Box[any]{Value: []int{1}}).Equal(Box[any]{Value: []int{1}})
}
