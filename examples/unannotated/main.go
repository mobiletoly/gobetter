package main

import "fmt"

func main() {
	v := NewModelBuilder().Name("upstream").ID("model-1").Values([2]int{1, 2}).Labels(map[string]string{"source": "schema"}).Build()
	fmt.Println(v.Name, v.ID(), v.Values, v.Labels["source"])
}
