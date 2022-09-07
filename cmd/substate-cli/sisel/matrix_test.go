package sisel

import "testing"

func expect_eq[T comparable](t *testing.T, expected, got T) {
	if expected != got {
		t.Errorf("invalid value, expected %v, got %v", expected, got)
	}
}

func TestTriangleCreate(t *testing.T) {
	m := NewTriangleMatrix[int](5)
	expect_eq(t, 5, m.Rows())
	expect_eq(t, 5, m.Columns())
}

func TestSetAndGet(t *testing.T) {
	m := NewTriangleMatrix[int](5)
	m.Set(0, 0, 5)
	expect_eq(t, 5, m.Get(0, 0))
	m.Set(0, 0, 6)
	expect_eq(t, 6, m.Get(0, 0))
	m.Set(1, 0, 7)
	expect_eq(t, 6, m.Get(0, 0))
	expect_eq(t, 7, m.Get(1, 0))
	m.Set(4, 4, 8)
	expect_eq(t, 6, m.Get(0, 0))
	expect_eq(t, 7, m.Get(1, 0))
	expect_eq(t, 8, m.Get(4, 4))
}
