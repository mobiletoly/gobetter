package main

//gobetter:struct builder
type Box[T any] struct {
	Value T
	Fixed [2]T
}

// Go 1.26 self-referencing constraint; generated binders preserve this identity.
//
//gobetter:struct builder
type Chain[T interface{ Next() T }] struct{ Value T }
type Link int

func (v Link) Next() Link { return v + 1 }
