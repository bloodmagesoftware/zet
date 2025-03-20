package util

import "os"

func Exists(name string) bool {
	_, err := os.Stat(name)
	return err == nil
}

func SlicesFilter[T any](v []T, filter func(T) bool) []T {
	nv := make([]T, 0, len(v))
	for _, el := range v {
		if filter(el) {
			nv = append(nv, el)
		}
	}
	return nv
}
