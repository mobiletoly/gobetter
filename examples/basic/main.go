package main

import (
	"fmt"
	"time"
)

func main() {
	p := NewPersonBuilder().Name("Ada").DOB(time.Date(1815, 12, 10, 0, 0, 0, 0, time.UTC)).Comment("optional").Build()
	fmt.Println(p.Name, p.DOB().Format(time.DateOnly), p.Comment)
	window := NewWindowedPersonBuilder().Note("initial").FirstName("Ada")
	done := window.Alias("Ada L").Comment("first").DOB(p.DOB())
	window.Comment("late") // A retained stage still mutates the shared root.
	windowed := done.Suffix("end").Build()
	fmt.Println(windowed.FirstName, windowed.Note, windowed.Comment, windowed.Suffix)
}
