package sisel

import "fmt"

type Matrix[T any] interface {
	Get(i, j int) T
	Set(i, j int, value T)
	Rows() int
	Columns() int
}

type Triangle[T any] struct {
	rows int
	data []T
}

func NewTriangleMatrix[T any](rows int) Triangle[T] {
	return Triangle[T]{rows: rows, data: make([]T, triangleSize(rows))}
}

func (t *Triangle[T]) Get(i, j int) T {
	if j > i || i >= t.rows {
		panic(fmt.Sprintf("Can not access element %d,%d in triangle matrix with %d rows", i, j, t.rows))
	}
	pos := triangleSize(i) + j
	return t.data[pos]
}

func (t *Triangle[T]) Set(i, j int, value T) {
	if j > i || i >= t.rows {
		panic(fmt.Sprintf("Can not access element %d,%d in triangle matrix with %d rows", i, j, t.rows))
	}
	pos := triangleSize(i) + j
	t.data[pos] = value
}

func (t *Triangle[T]) Rows() int {
	return t.rows
}

func (t *Triangle[T]) Columns() int {
	return t.Rows()
}

func triangleSize(rows int) int {
	return ((rows + 1) * rows) / 2
}
