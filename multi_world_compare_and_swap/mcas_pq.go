package multi_world_compare_and_swap

import (
	"cmp"
	"errors"
	"slices"
	"sync/atomic"
	"unsafe"
)

// MCASDescriptor ref: https://arxiv.org/pdf/2008.02527
type MCASDescriptor[V any] struct {
	status atomic.Int32
	N      int
	words  [2]WordDescriptor[V]
}

type MCASWord[V any] struct {
	value V
	idx   int
	desc  *MCASDescriptor[V]
}

type WordDescriptor[V any] struct {
	addr *atomic.Pointer[MCASWord[V]]
	old  *MCASWord[V]
	new  *MCASWord[V]

	selfWord MCASWord[V]
}

const (
	ACTIVE int32 = iota
	SUCCESSFUL
	FAIL
)

var (
	ErrNumOfWords      = errors.New("multi_world_compare_and_swap: too many words")
	ErrNilAddress      = errors.New("multi_world_compare_and_swap: nil address")
	ErrMustBeValueWord = errors.New("multi_world_compare_and_swap: new must be a value word")
	ErrDuplicateAddr   = errors.New("multi_world_compare_and_swap: duplicate address")
)

func NewMCASDescriptor[V any](words ...WordDescriptor[V]) (*MCASDescriptor[V], error) {
	d := &MCASDescriptor[V]{N: len(words)}

	if len(words) == 0 || len(words) > len(d.words) {
		return nil, ErrNumOfWords
	}

	copy(d.words[:], words)

	ws := d.words[:d.N]

	slices.SortFunc(ws, func(a, b WordDescriptor[V]) int {
		return cmp.Compare(uintptr(unsafe.Pointer(a.addr)), uintptr(unsafe.Pointer(b.addr)))
	})

	for i := range ws {
		w := &ws[i]
		switch {
		case w.addr == nil:
			return nil, ErrNilAddress
		case w.new != nil && w.new.desc != nil:
			return nil, ErrMustBeValueWord
		case i > 0 && ws[i-1].addr == ws[i].addr:
			return nil, ErrDuplicateAddr
		default:
			w.selfWord = MCASWord[V]{desc: d, idx: i}
		}
	}
	return d, nil
}
