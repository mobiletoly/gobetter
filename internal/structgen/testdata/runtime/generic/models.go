// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

package models

type gobetterT0 int
type gobetterArg int
type gobetterResult int
type OuterConfig int

var new = 1

type Base[T any] struct{ Value T }

//gobetter:struct builder
type Outer[Outer ~int | ~int64] struct {
	Base[Outer]
	Config struct {
		Value Outer `json:"value"`
		Deep  *struct {
			Value Outer `json:"deep"`
		}
	}
	Other *struct {
		Value Outer `json:"outer"`
	}
	//gobetter:field getter,skip-builder
	hidden    Outer
	Call      func(Outer int) Outer
	Array     [len([2]func(){func() { Outer := 3; _ = Outer }})]Outer
	Collision gobetterArg
}

type Self int

func (v Self) Self() Self { return v }

//gobetter:struct builder
type Box[Box interface{ Self() Box }] struct{ Value Box }
