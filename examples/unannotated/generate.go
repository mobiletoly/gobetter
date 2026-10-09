// Command unannotated demonstrates external configuration of annotation-free structs.
package main

//go:generate go run ./testdata/upstream/main.go
//go:generate go run ../../cmd/gobetter generate -config=gobetter.json .
