package main

import "fmt"

func main() {
	v := NewRecordBuilder().Address(Address{City: "London"}).Owner(&Owner{Name: "Ada"}).Envelope(Envelope[int]{Value: 7}).Build()
	fmt.Println(v.City, v.Name, v.Value)
}
