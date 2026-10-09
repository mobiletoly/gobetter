package main

import "fmt"

func main() {
	b := NewBoxBuilder[string]().Value("hello").Fixed([2]string{"one", "two"}).Build()
	c := NewChainBuilder[Link]().Value(2).Build()
	fmt.Println(b.Value, b.Fixed, c.Value.Next())
}
