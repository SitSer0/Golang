//go:build !solution

package genericsum

import (
	"golang.org/x/exp/constraints"
	"math/cmplx"
	"sort"
	"sync"
)

func Min[K constraints.Ordered](a, b K) K {
	if a < b {
		return a
	}
	return b
}

func SortSlice[T constraints.Ordered](s []T) {
	sort.Slice(s, func(i, j int) bool {
		return s[i] < s[j]
	})
}

func MapsEqual[K comparable, V comparable](a, b map[K]V) bool {
	for k, v := range a {
		val, err := b[k]
		if err != true || val != v {
			return false
		}
	}

	for k, v := range b {
		val, err := a[k]
		if err != true || val != v {
			return false
		}
	}
	return true
}

func SliceContains[V comparable](s []V, v V) bool {
	for _, val := range s {
		if val == v {
			return true
		}
	}
	return false
}

func MergeChans[T any](chs ...<-chan T) <-chan T {
	out := make(chan T)

	var wg sync.WaitGroup
	wg.Add(len(chs))

	for _, ch := range chs {
		go func(c <-chan T) {
			for v := range c {
				out <- v
			}
			wg.Done()
		}(ch)
	}

	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr |
		~float32 | ~float64 |
		~complex64 | ~complex128
}

func IsHermitianMatrix[V Number](m [][]V) bool {
	n := len(m)
	for i := 0; i < n; i++ {
		for j := i; j < n; j++ {
			switch a := any(m[i][j]).(type) {
			case complex64:
				b := any(m[j][i]).(complex64)
				if a != complex64(cmplx.Conj(complex128(b))) {
					return false
				}
			case complex128:
				b := any(m[j][i]).(complex128)
				if a != cmplx.Conj(b) {
					return false
				}
			default:
				if m[i][j] != m[j][i] {
					return false
				}
			}
		}
	}
	return true
}
