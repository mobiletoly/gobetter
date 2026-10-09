// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

package models

import formatter "fmt"

type gobetterFmt int
type gobetterSlices int
type gobetterMaps int
type gobetterReceiver int
type gobetterOther int
type Number int
type Fixed [2]Number
type Value struct {
	Number Number
	Array  Fixed
}

func (Value) Equal(Value) bool { panic("user equality must not run") }

type Values []Value
type Lookup map[string]Value
type Bad struct {
	Dynamic  any
	Function func()
}
type embed struct{ X int }

//gobetter:struct equal,string
type Record struct {
	_         int
	Scalar    Number
	Array     Fixed
	Aggregate Value
	Slice     Values
	Map       Lookup
	Pointer   *Bad
	Channel   chan int
	private   int
	//gobetter:field skip-equal,skip-string
	Function func()
	//gobetter:field skip-equal,skip-string
	Dynamic any
}

//gobetter:struct string
type Display struct {
	Z int
	a string
	embed
	//gobetter:field skip-string
	Hidden  int
	Printer formatter.Stringer
}

//gobetter:struct equal,string
type Empty struct{}

//gobetter:struct equal,string
type Skipped struct {
	//gobetter:field skip-equal,skip-string
	Unsupported func()
}

//gobetter:struct equal
type Float struct{ Value float64 }

//gobetter:struct equal,string
type Box[Box comparable] struct{ Value Box }

type Ints []int
type Ints2 []int
type SliceTerms interface{ Ints | Ints2 }

//gobetter:struct equal
type GenericSlice[E comparable, S interface{ ~[]E }] struct{ Values S }

//gobetter:struct equal
type GenericUnion[S SliceTerms] struct{ Values S }

//gobetter:struct equal
type GenericMap[K comparable, V comparable, M ~map[K]V] struct{ Values M }

//gobetter:struct builder,equal,string,getters
type Mixed struct {
	//gobetter:field getter,skip-builder
	hidden int
	Value  int
	//gobetter:field optional
	Note string
}

//gobetter:struct equal
type Pair[T comparable] struct {
	Z T
	A int
}

//gobetter:struct equal
type ShortCircuit[T comparable] struct {
	A int
	Z T
}

//gobetter:struct equal
type FilteredPair[T comparable] struct {
	Ignored T //gobetter:field skip-equal
	A       int
	Z       T
}
