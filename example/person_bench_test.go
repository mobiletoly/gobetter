package example

import (
	"testing"
	"time"
)

var (
	sink *Person
	tDOB = time.Date(1990, 7, 1, 0, 0, 0, 0, time.UTC)
)

func BenchmarkDirectLiteral(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		p := &Person{
			firstName: "John",
			lastName:  "Doe",
			dob:       tDOB,
			Email:     "john@example.com",
			// Phone, Bio left nil (optional)
		}
		// prevent compiler from optimizing it away
		sink = p
	}
}

func BenchmarkBuilderChain(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		p := NewPersonBuilder().
			DOB(tDOB).
			Email("john@example.com").
			FirstName("John").
			LastName("Doe").
			Build()
		sink = p
	}
}
