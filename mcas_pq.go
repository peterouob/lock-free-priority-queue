package pq

import (
	"fmt"
	"slices"
	"sort"
	"sync/atomic"
	"unsafe"
)

// MCASDescriptor ref: https://arxiv.org/pdf/2008.02527
type MCASDescriptor[V any] struct {
	status atomic.Int32
	N      int
	words  [2]WordDescriptor[V]
}

type WordDescriptor[V any] struct {
	addr *atomic.Pointer[Word[V]]
	old  *Word[V]
	new  *Word[V]

	selfWord Word[V]
}

const (
	ACTIVE int32 = iota
	SUCCESSFUL
	FAIL
)

func NewMCASDescriptor[V any](words ...WordDescriptor[V]) (*MCASDescriptor[V], error) {
	raddr := uintptr(unsafe.Pointer(words[0].addr))

	cpWords := make([]WordDescriptor[V], len(words)-1)
	copy(cpWords, words[1:])

	for word := range slices.Values(cpWords) {
		addr := uintptr(unsafe.Pointer(word.addr))
		if addr == raddr {
			return nil, fmt.Errorf("duplicate address: %p", word.addr)
		}

		raddr = addr
	}

	sort.Slice(words, func(i, j int) bool {
		return uintptr(unsafe.Pointer(words[i].addr)) < uintptr(unsafe.Pointer(words[j].addr))
	})

	d := &MCASDescriptor[V]{N: len(words)}
	for i := range d.N {
		d.words[i] = words[i]
		d.words[i].selfWord.desc = d
	}

	return d, nil
}

func (m *MCASDescriptor[V]) Complete() {}
