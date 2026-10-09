// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

package models

var new = 1

type gobetterResult int
type embedding struct{ V int }
type PtrEmbedded struct{ V int }

//gobetter:struct builder
type Record struct {
	_      int
	A      int
	B      string
	Shared []int
	Map    map[string]int
	Ref    *int
	*PtrEmbedded
	//gobetter:field getter
	embedding
	//gobetter:field getter,skip-builder
	secret int
	//gobetter:field optional
	Note string
}

//gobetter:struct builder
type Empty struct{}

//gobetter:struct builder
type OnlyOptional struct {
	//gobetter:field optional
	A int
	//gobetter:field optional
	B int
}

//gobetter:struct builder
type OnlySkipped struct {
	//gobetter:field getter,skip-builder
	hidden int
}

//gobetter:struct getters
type GetterOnly struct {
	//gobetter:field getter
	hidden int
}

//gobetter:struct builder
type privateType struct{ Value int }

//gobetter:struct builder,constructor=package
type Public struct{ Value int }

//gobetter:struct builder,constructor=exported
type privateExport struct{ Value int }

//gobetter:struct builder
type Terminals struct {
	Build    int
	BuildPtr int
	Finalize int
	Root     int
}

//gobetter:struct builder
type RenamedOptional struct {
	//gobetter:field optional,name=WithBuild
	Build int
}
