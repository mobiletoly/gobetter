package main

import "testing"

func TestEmbeddedAssignments(t *testing.T) {
	owner := &Owner{Name: "Ada"}
	v := NewRecordBuilder().Address(Address{City: "London"}).Owner(owner).Envelope(Envelope[int]{Value: 7}).Build()
	if v.City != "London" || v.Value != 7 || v.Owner != owner {
		t.Fatal(v)
	}
	owner.Name = "Grace"
	if v.Name != "Grace" {
		t.Fatal("pointer field should remain shared")
	}
}
