package pkg

import (
	"errors"
)

// Map func that appends slices
func Map[SRC, DST any](srcs []SRC, mapper func(SRC) DST) []DST {
	ret := make([]DST, 0)
	for _, src := range srcs {
		ret = append(ret, mapper(src))
	}

	return ret
}

// MapMap func that compare and appends slices
func MapMap[KEY comparable, VALUE, DST any](srcs map[KEY]VALUE, mapper func(KEY, VALUE) DST) []DST {
	ret := make([]DST, 0)
	for k, v := range srcs {
		ret = append(ret, mapper(k, v))
	}

	return ret
}

// MapKeys func that compare and appends slices
func MapKeys[K comparable, V any](m map[K]V) []K {
	ret := make([]K, 0)
	for k := range m {
		ret = append(ret, k)
	}

	return ret
}

// Prepend func that return two append items
func Prepend[V any](array []V, items ...V) []V {
	return append(items, array...)
}

// ContainValue func that valid if contains value
func ContainValue[V comparable](array []V, value V) bool {
	for _, v := range array {
		if v == value {
			return true
		}
	}

	return false
}

// ErrOpenningBrowser is a struct that return an error when opening browser
var ErrOpenningBrowser = errors.New("opening browser")
