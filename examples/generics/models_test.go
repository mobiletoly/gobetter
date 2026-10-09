package main

import "testing"

func TestGenericIdentityAndSnapshots(t *testing.T) {
	early := NewBoxBuilder[string]()
	final := early.Value("first").Fixed([2]string{"a", "b"})
	v := final.Build()
	p := final.BuildPtr()
	early.Value("later")
	if v.Value != "first" || p.Value != "first" || v.Fixed != ([2]string{"a", "b"}) {
		t.Fatal(v, p)
	}
	c := NewChainBuilder[Link]().Value(3).Build()
	if c.Value.Next() != 4 {
		t.Fatal(c)
	}
}
