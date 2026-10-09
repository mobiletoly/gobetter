package models

import (
	"reflect"
	"testing"
)

func TestNestedGenericAliasesAndSnapshots(t *testing.T) {
	deep := NewOuterConfigDeepBuilder[int]().Value(2).BuildPtr()
	config := NewOuterConfig_2Builder[int]().Value(1).Deep(deep).Build()
	other := NewOuterOtherBuilder[int]().Value(3).BuildPtr()
	value := NewOuterBuilder[int]().Base(Base[int]{Value: 4}).Config(config).Other(other).Call(func(v int) int { return v + 1 }).Array([2]int{5, 6}).Collision(7).Build()
	if value.Base.Value != 4 || value.Config.Value != 1 || value.Config.Deep.Value != 2 || value.Other.Value != 3 || value.Call(8) != 9 || value.Array[1] != 6 || value.Collision != 7 || value.Hidden() != 0 {
		t.Fatal("generic/nested/embedded type or assignment lost")
	}
	field, _ := reflect.TypeOf(value.Config).FieldByName("Value")
	if field.Tag.Get("json") != "value" {
		t.Fatal("alias tag/identity lost")
	}
	if NewBoxBuilder[Self]().Value(9).Build().Value.Self() != 9 {
		t.Fatal("self-referencing generic constraint lost")
	}
	if new != 1 {
		t.Fatal("shadowed builtin changed")
	}
}
