package logs

import (
	"strings"
	"testing"
)

func TestRingWriteAndLines(t *testing.T) {
	r := NewRing(3)
	r.Write("a")
	r.Write("b")
	r.Write("c")
	got := r.Lines()
	if strings.Join(got, ",") != "a,b,c" {
		t.Errorf("Lines = %v, want [a b c]", got)
	}
}

func TestRingEviction(t *testing.T) {
	r := NewRing(3)
	for _, s := range []string{"a", "b", "c", "d", "e"} {
		r.Write(s)
	}
	got := r.Lines()
	if strings.Join(got, ",") != "c,d,e" {
		t.Errorf("Lines = %v, want [c d e]", got)
	}
}

func TestRingEmpty(t *testing.T) {
	r := NewRing(3)
	if got := r.Lines(); len(got) != 0 {
		t.Errorf("Lines = %v, want empty", got)
	}
}

func TestRingCapacityOne(t *testing.T) {
	r := NewRing(1)
	r.Write("a")
	r.Write("b")
	if got := r.Lines(); len(got) != 1 || got[0] != "b" {
		t.Errorf("Lines = %v, want [b]", got)
	}
}
