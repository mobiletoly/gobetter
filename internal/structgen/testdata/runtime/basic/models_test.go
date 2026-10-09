package models

import "testing"

func TestSnapshotsAndReferenceSharing(t *testing.T) {
	shared := []int{2}
	lookup := map[string]int{"x": 3}
	ref := 4
	embedded := &PtrEmbedded{V: 5}
	first := NewRecordBuilder()
	done := first.A(1).B("old").Shared(shared).Map(lookup).Ref(&ref).PtrEmbedded(embedded).Embedding(embedding{V: 6})
	value := done.Build()
	ptr := done.BuildPtr()
	other := done.BuildPtr()
	first.A(9)
	done.Note("later")
	if value.A != 1 || ptr.A != 1 || other.A != 1 || value.Note != "" || ptr.Note != "" {
		t.Fatal("completed result aliases builder root")
	}
	if done.Build().A != 9 || done.Build().Note != "later" {
		t.Fatal("retained steps did not share current root")
	}
	ptr.A = 99
	if other.A != 1 || done.Build().A != 9 || ptr == other {
		t.Fatal("BuildPtr did not create independent nonzero copies")
	}
	shared[0] = 20
	lookup["x"] = 30
	ref = 40
	embedded.V = 50
	if value.Shared[0] != 20 || other.Map["x"] != 30 || *value.Ref != 40 || other.PtrEmbedded.V != 50 {
		t.Fatal("snapshot deep-copied reference fields")
	}
	if value.Embedding().V != 6 || value.Secret() != 0 {
		t.Fatal("independent getters wrong")
	}
	var absent *Record
	defer func() {
		if recover() == nil {
			t.Fatal("getter unexpectedly guards nil receiver")
		}
	}()
	absent.Secret()
}

func TestZeroRequiredVisibilityAndTerminalNames(t *testing.T) {
	_ = NewEmptyBuilder().Build()
	_ = NewEmptyBuilder().BuildPtr()
	value := NewOnlyOptionalBuilder().A(1).A(2).B(3).Build()
	if value.A != 2 || value.B != 3 || NewOnlyOptionalBuilder().Build().A != 0 {
		t.Fatal("optional final-stage semantics wrong")
	}
	skipped := NewOnlySkippedBuilder().Build()
	if skipped.Hidden() != 0 {
		t.Fatal("skipped getter wrong")
	}
	only := GetterOnly{hidden: 7}
	if only.Hidden() != 7 {
		t.Fatal("getter-only result wrong")
	}
	if newPrivateTypeBuilder().Value(8).Build().Value != 8 || newPublicBuilder().Value(9).Build().Value != 9 || NewPrivateExportBuilder().Value(10).Build().Value != 10 {
		t.Fatal("constructor visibility/settings wrong")
	}
	terminals := NewTerminalsBuilder().Build(1).BuildPtr(2).Finalize(3).Root(4).Build()
	if terminals.Build != 1 || terminals.BuildPtr != 2 || terminals.Finalize != 3 || terminals.Root != 4 {
		t.Fatal("required terminal spelling collided")
	}
	if NewRenamedOptionalBuilder().WithBuild(8).Build().Build != 8 {
		t.Fatal("explicit optional terminal override wrong")
	}
}
