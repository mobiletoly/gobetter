package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestSnapshotsAndOutputs(t *testing.T) {
	birth := time.Date(1815, 12, 10, 0, 0, 0, 0, time.UTC)
	early := NewPersonBuilder()
	final := early.Name("Ada").DOB(birth)
	v := final.Comment("first").Build()
	p := final.Comment("second").BuildPtr()
	early.Name("Grace")
	if v.Name != "Ada" || p.Name != "Ada" || v.Comment != "first" || p.Comment != "second" || v.DOB() != birth {
		t.Fatal(v, p)
	}
	cloned := v
	if !v.Equal(cloned) || !strings.Contains(v.String(), `Name:"Ada"`) {
		t.Fatal(v.String())
	}
	cloned.Comment = "changed"
	if v.Equal(cloned) {
		t.Fatal("excluded fields or equality incorrect")
	}
	zero := NewPersonBuilder().Name("Ada").DOB(birth).Build()
	if zero.Comment != "" {
		t.Fatal("optional setter required")
	}
}

func TestSourcePositionOptionalWindows(t *testing.T) {
	birth := time.Date(1815, 12, 10, 0, 0, 0, 0, time.UTC)
	initial := NewWindowedPersonBuilder()
	window := initial.Note("old").Note("initial").FirstName("Ada")
	done := window.Alias("old").Comment("first").Alias("alias").DOB(birth)
	before := done.BuildPtr()
	initial.Note("late leading")
	window.Comment("late middle")
	value := done.Suffix("old").Suffix("suffix").Build()
	if before.Note != "initial" || before.Comment != "first" || value.Note != "late leading" || value.Comment != "late middle" || value.Alias != "alias" || value.Suffix != "suffix" || value.Cache != 0 || value.DOB() != birth {
		t.Fatal(before, value)
	}
	omitted := NewWindowedPersonBuilder().FirstName("Ada").DOB(birth).Build()
	if omitted.Note != "" || omitted.Comment != "" || omitted.Alias != "" || omitted.Suffix != "" {
		t.Fatal(omitted)
	}
}

type readCloser struct {
	*bytes.Reader
	closed bool
}

func (r *readCloser) Close() error { r.closed = true; return nil }

type alternative struct{}

func (alternative) Read(p []byte) (int, error) { return copy(p, "hello"), nil }
func (alternative) Close() error               { return nil }
func TestInlineInterfaceBuilders(t *testing.T) {
	a := &readCloser{Reader: bytes.NewReader([]byte("data"))}
	for _, reader := range []interface {
		Read([]byte) (int, error)
		Close() error
	}{a, alternative{}} {
		v := NewStreamBuilder().Identifier("stream").IO(reader).Build()
		buf := make([]byte, 5)
		n, err := v.IO.Read(buf)
		if err != nil || n == 0 || v.Identifier != "stream" {
			t.Fatal(v, n, err)
		}
		if err := v.IO.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if !a.closed {
		t.Fatal("interface binding lost")
	}
}
