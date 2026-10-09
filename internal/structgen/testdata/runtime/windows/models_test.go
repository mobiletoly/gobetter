package models

import "testing"

func TestOptionalWindows(t *testing.T) {
	value := NewWindowBuilder().Leading("old").Leading("lead").First(1).
		Alias("old").Middle("middle").Alias("alias").Next("next").Tail("old").Tail("tail").Build()
	if value.Leading != "lead" || value.Middle != "middle" || value.Alias != "alias" || value.Tail != "tail" || value.Next() != "next" || value.Ignored != 0 {
		t.Fatal(value)
	}
	reversed := NewWindowBuilder().First(2).Middle("middle").Alias("alias").Next("next").BuildPtr()
	if reversed.First != 2 || reversed.Middle != "middle" || reversed.Alias != "alias" {
		t.Fatal(reversed)
	}
	omitted := NewWindowBuilder().First(3).Next("next").Build()
	if omitted.Leading != "" || omitted.Middle != "" || omitted.Alias != "" || omitted.Tail != "" {
		t.Fatal("omitted optional fields did not keep zero values", omitted)
	}
	initial := NewWindowBuilder()
	window := initial.First(4)
	done := window.Next("next")
	before := done.Build()
	ptr := done.BuildPtr()
	initial.Leading("late leading")
	window.Middle("late middle").Alias("late alias")
	done.Tail("late tail")
	after := done.Build()
	if after.Leading != "late leading" || after.Middle != "late middle" || after.Alias != "late alias" || after.Tail != "late tail" {
		t.Fatal("retained stages lost shared-root mutation", after)
	}
	if before.Leading != "" || before.Middle != "" || ptr.Leading != "" || ptr.Tail != "" {
		t.Fatal("Build and BuildPtr snapshots alias the builder root", before, ptr)
	}
	all := NewOptionalBuilder().B("b").A("old").A("a").Build()
	if all.A != "a" || all.B != "b" || NewOptionalBuilder().BuildPtr().A != "" || NewSkippedBuilder().Build().Value != 0 {
		t.Fatal("zero-required builder availability or values changed", all)
	}
}
