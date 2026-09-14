package pq

import (
	"sort"
	"sync/atomic"
	"unsafe"
)

type descriptor interface {
	Complete()
}

type dcssDescriptor interface {
	descriptor

	dcssDescriptor()
}

type Word[V any] struct {
	value V
	desc  descriptor
}

func priority(w *Word[CMNode]) uint32 {
	if w == nil || w.value.list == nil {
		return emptyPriority
	}

	return w.value.list.value.priority
}

const (
	UNDECIDED int32 = iota
	SUCCEEDED
	FAILED
)

type DcssDescriptor[C, V any] struct {
	a1 *atomic.Pointer[Word[C]] // control address
	o1 *Word[C]                 // old value

	a2 *atomic.Pointer[Word[V]] // address
	o2 *Word[V]                 // expect old value
	n2 *Word[V]                 // new value

	self *Word[V]

	status atomic.Int32
}

func NewDcssDescriptor[C, V any](
	a1 *atomic.Pointer[Word[C]], o1 *Word[C],
	a2 *atomic.Pointer[Word[V]], o2 *Word[V],
	n2 *Word[V],
) *DcssDescriptor[C, V] {
	d := &DcssDescriptor[C, V]{a1: a1, o1: o1, a2: a2, o2: o2, n2: n2}
	d.self = &Word[V]{desc: d}
	return d
}

func (d *DcssDescriptor[C, V]) dcssDescriptor() {}

func (d *DcssDescriptor[C, V]) Dcss() *Word[V] {
	for {
		r := d.a2.Load()
		if isDesc, ok := r.desc.(dcssDescriptor); ok {
			isDesc.Complete()
			continue
		}
		if r != d.o2 {
			return r
		}
		if d.a2.CompareAndSwap(d.o2, d.self) {
			d.Complete()
			return d.o2
		}
	}
}

func (d *DcssDescriptor[C, V]) Complete() {
	s := d.status.Load()

	if s == UNDECIDED {
		decision := SUCCEEDED
		if DcssRead(d.a1) != d.o1 {
			decision = FAILED
		}
		d.status.CompareAndSwap(UNDECIDED, decision)
		s = d.status.Load()
	}

	if s == SUCCEEDED {
		d.a2.CompareAndSwap(d.self, d.n2)
		return
	}

	d.a2.CompareAndSwap(d.self, d.o2)
}

func DcssRead[V any](addr *atomic.Pointer[Word[V]]) *Word[V] {
	for {
		r := addr.Load()
		isDesc, ok := r.desc.(dcssDescriptor)
		if !ok {
			return r
		}

		isDesc.Complete()
	}
}

var (
	CasnUndecided = &Word[int32]{value: UNDECIDED}
	CasnSucceeded = &Word[int32]{value: SUCCEEDED}
	CasnFailed    = &Word[int32]{value: FAILED}
)

type CasnEntry[V any] struct {
	addr *atomic.Pointer[Word[V]]
	old  *Word[V]
	new  *Word[V]
}

func NewCasnEntry[V any](addr *atomic.Pointer[Word[V]], old, new *Word[V]) CasnEntry[V] {
	return CasnEntry[V]{addr: addr, old: old, new: new}
}

type CasnDescriptor[V any] struct {
	status  atomic.Pointer[Word[int32]]
	entries []CasnEntry[V]

	self *Word[V]
}

func NewCasnDescriptor[V any](entries ...CasnEntry[V]) *CasnDescriptor[V] {
	sort.Slice(entries, func(i, j int) bool {
		return uintptr(unsafe.Pointer(entries[i].addr)) < uintptr(unsafe.Pointer(entries[j].addr))
	})

	c := &CasnDescriptor[V]{entries: entries}
	c.status.Store(CasnUndecided)
	c.self = &Word[V]{desc: c}
	return c
}

func (cd *CasnDescriptor[V]) Complete() {}

func (cd *CasnDescriptor[V]) Casn() bool {
	if cd.status.Load() == CasnUndecided {
		status := CasnSucceeded

		for i := 0; i < len(cd.entries) && status == CasnSucceeded; i++ {
		retry:
			e := cd.entries[i]
			d := NewDcssDescriptor(&cd.status, CasnUndecided, e.addr, e.old, cd.self)

			switch val := d.Dcss(); {
			case val == cd.self:
			case val.desc != nil:
				if isCasn, ok := val.desc.(*CasnDescriptor[V]); ok {
					isCasn.Casn()
					goto retry
				}
			case val != e.old:
				status = CasnFailed
			}
		}

		cd.status.CompareAndSwap(CasnUndecided, status)
	}

	succeeded := cd.status.Load() == CasnSucceeded
	for _, e := range cd.entries {
		if succeeded {
			e.addr.CompareAndSwap(cd.self, e.new)
			continue
		}
		e.addr.CompareAndSwap(cd.self, e.old)
	}

	return succeeded
}

func CasnRead[V any](addr *atomic.Pointer[Word[V]]) *Word[V] {
	for {
		r := DcssRead(addr)
		isCasn, ok := r.desc.(*CasnDescriptor[V])
		if !ok {
			return r
		}

		isCasn.Casn()
	}
}
