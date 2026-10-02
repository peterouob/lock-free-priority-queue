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
	desc  *WordDescriptor[V]
}

type WordDescriptor[V any] struct {
	addr *atomic.Pointer[MCASWord[V]]
	old  *MCASWord[V]
	new  *MCASWord[V]

	parent *MCASDescriptor[V]

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
		case (w.new != nil && w.new.desc != nil) ||
			(w.old != nil && w.old.desc != nil):
			return nil, ErrMustBeValueWord
		case i > 0 && ws[i-1].addr == ws[i].addr:
			return nil, ErrDuplicateAddr
		default:
			w.parent = d
			w.selfWord = MCASWord[V]{desc: w}
		}
	}
	return d, nil
}

func (m *MCASDescriptor[V]) readInternal(addr *atomic.Pointer[MCASWord[V]]) (*MCASWord[V], *MCASWord[V]) {
	for {
		val := addr.Load()

		if val == nil || val.desc == nil {
			return val, val
		}

		parent := val.desc.parent

		status := parent.status.Load()

		if parent != m && status == ACTIVE {
			m.MCAS(parent)
			continue
		}

		if status == SUCCESSFUL {
			return val, val.desc.new
		}

		return val, val.desc.old
	}
}

func Read[V any](addr *atomic.Pointer[MCASWord[V]]) *MCASWord[V] {
	_, v := (*MCASDescriptor[V])(nil).readInternal(addr)

	return v
}

func (m *MCASDescriptor[V]) MCAS(desc *MCASDescriptor[V]) bool {
	success := true
	for i := range m.words {
		word := m.words[i]
	retry_word:
		content, value := m.readInternal(word.addr)

		if content == &word.selfWord {
			continue
		}

		if value != word.old {
			success = false
			break
		}

		status := desc.status.Load()

		if status != ACTIVE {
			break
		}

		if !word.addr.CompareAndSwap(content, &word.selfWord) {
			goto retry_word
		}

		status = SUCCESSFUL
		if !success {
			status = FAIL
		}

		if desc.status.CompareAndSwap(ACTIVE, status) {
			return desc.status.Load() == SUCCESSFUL
		}
	}
	return false
}
