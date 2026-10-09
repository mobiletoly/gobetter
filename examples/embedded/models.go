package main

type Address struct{ City string }
type Owner struct{ Name string }
type Envelope[T any] struct{ Value T }

//gobetter:struct builder
type Record struct {
	Address
	*Owner
	Envelope[int]
}
