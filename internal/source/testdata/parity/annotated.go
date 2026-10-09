// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

package models

//gobetter:struct builder,equal,string,constructor=package
type Record struct {
	Z int
	//gobetter:field name=ID,getter,optional,skip-string
	id string
	//gobetter:field skip-builder,skip-equal
	data func()
	A, B bool
	_    int
}
