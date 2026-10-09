package main

import "time"

//gobetter:struct builder,equal,string
type Person struct {
	// Name of the person
	Name string
	// Date of birth
	//gobetter:field getter,name=DOB
	dob time.Time
	// Optional comments
	//gobetter:field optional
	Comment string
}

//gobetter:struct builder
type WindowedPerson struct {
	// Note is available before FirstName.
	Note string //gobetter:field optional
	// First name of the person
	FirstName string
	// Optional comments in the window before DOB.
	Comment string //gobetter:field optional
	Alias   string //gobetter:field optional
	// Date of birth
	dob time.Time //gobetter:field getter,name=DOB
	// Suffix is available alongside Build and BuildPtr.
	Suffix string //gobetter:field optional
	Cache  int    //gobetter:field skip-builder
}

//gobetter:struct builder
type Point struct {
	X int
	Y int
}

//gobetter:struct builder
type Large struct {
	Numbers [8]int64
	Payload [256]byte
	Name    string
}

//gobetter:struct builder
type Stream struct {
	Identifier string
	IO         interface {
		Read([]byte) (int, error)
		Close() error
	}
}
