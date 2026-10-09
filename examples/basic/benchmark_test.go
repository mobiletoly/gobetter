package main

import "testing"

var pointValue Point
var pointPointer *Point
var largeValue Large
var largePointer *Large

func BenchmarkDirectValue(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		pointValue = Point{X: 1, Y: 2}
	}
}
func BenchmarkBuilderValue(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		pointValue = NewPointBuilder().X(1).Y(2).Build()
	}
}
func BenchmarkDirectPointer(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		v := Point{X: 1, Y: 2}
		pointPointer = &v
	}
}
func BenchmarkBuilderPointer(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		pointPointer = NewPointBuilder().X(1).Y(2).BuildPtr()
	}
}
func BenchmarkDirectRetainedValue(b *testing.B) {
	b.ReportAllocs()
	v := Point{}
	for b.Loop() {
		v.X = 1
		v.Y = 2
		pointValue = v
	}
}
func BenchmarkBuilderRetainedValue(b *testing.B) {
	b.ReportAllocs()
	early := NewPointBuilder()
	for b.Loop() {
		pointValue = early.X(1).Y(2).Build()
	}
}
func BenchmarkDirectRetainedPointer(b *testing.B) {
	b.ReportAllocs()
	v := Point{}
	for b.Loop() {
		v.X = 1
		v.Y = 2
		cloned := v
		pointPointer = &cloned
	}
}
func BenchmarkBuilderRetainedPointer(b *testing.B) {
	b.ReportAllocs()
	early := NewPointBuilder()
	for b.Loop() {
		pointPointer = early.X(1).Y(2).BuildPtr()
	}
}
func BenchmarkDirectLargeValue(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		largeValue = Large{Numbers: [8]int64{1}, Payload: [256]byte{2}, Name: "large"}
	}
}
func BenchmarkBuilderLargeValue(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		largeValue = NewLargeBuilder().Numbers([8]int64{1}).Payload([256]byte{2}).Name("large").Build()
	}
}
func BenchmarkDirectLargePointer(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		v := Large{Numbers: [8]int64{1}, Payload: [256]byte{2}, Name: "large"}
		largePointer = &v
	}
}
func BenchmarkBuilderLargePointer(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		largePointer = NewLargeBuilder().Numbers([8]int64{1}).Payload([256]byte{2}).Name("large").BuildPtr()
	}
}
