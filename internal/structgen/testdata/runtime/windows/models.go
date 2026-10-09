// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

package models

//gobetter:struct builder
type Window struct {
	// Leading note.
	Leading string //gobetter:field optional
	First   int
	Ignored int //gobetter:field skip-builder
	// Middle note.
	Middle string //gobetter:field optional
	Alias  string //gobetter:field optional
	// Next required value.
	next string //gobetter:field getter,name=Next
	Tail string //gobetter:field optional
}

//gobetter:struct builder
type Optional struct {
	A string //gobetter:field optional
	B string //gobetter:field optional
}

//gobetter:struct builder
type Skipped struct {
	Value int //gobetter:field skip-builder
}
