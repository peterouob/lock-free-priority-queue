package pq

import (
	"sync/atomic"
)

type Word[V any] struct {
	value V
	desc  *DcssDescriptor[V]
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

type DcssDescriptor[V any] struct {
	a1   *atomic.Pointer[Word[V]]
	o1   *Word[V]
	a2   *atomic.Pointer[Word[V]]
	o2   *Word[V]
	n2   *Word[V]
	self *Word[V]

	status atomic.Int32
}

func NewDcssDescriptor[V any](
	a1 *atomic.Pointer[Word[V]], o1 *Word[V],
	a2 *atomic.Pointer[Word[V]], o2 *Word[V],
	n2 *Word[V],
) *DcssDescriptor[V] {
	d := &DcssDescriptor[V]{a1: a1, o1: o1, a2: a2, o2: o2, n2: n2}
	d.self = &Word[V]{desc: d}
	return d
}

func (d *DcssDescriptor[V]) Dcss() *Word[V] {
	for {
		r := d.a2.Load()
		if r.desc != nil {
			r.desc.Complete()
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

func (d *DcssDescriptor[V]) Complete() {
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
		if r.desc == nil {
			return r
		}

		r.desc.Complete()
	}
}

type CasnDescriptor[T any] struct {
	n       int
	status  atomic.Int32
	entries []atomic.Pointer[entry[T]]
	self    *Word[T]
}

func NewCasnDescriptor[T any](n int) *CasnDescriptor[T] {
	c := &CasnDescriptor[T]{}
	c.status.Store(UNDECIDED)
	c.n = n
	c.entries = make([]atomic.Pointer[entry[T]], n)
	return c
}

type entry[V any] struct {
	a1 *atomic.Pointer[Word[V]]
	o1 *Word[V]
	a2 *atomic.Pointer[Word[V]]
	o2 *Word[V]
	n2 *Word[V]
}

func (cd *CasnDescriptor[T]) Casn() bool {
	status := cd.status.Load()
	// phase 1;
	for i := 0; (i < cd.n) && status == SUCCEEDED; i++ {
	retry:
		entry := cd.entries[i].Load()
		d := NewDcssDescriptor(entry.a1, entry.o1, entry.a2, entry.o2, entry.n2)
		val := d.Dcss()

		if val.desc != nil {
			if val != cd.self {
				cd.Casn()
				goto retry
			}
		} else if val != entry.o1 {
			status = FAILED
		}
		cd.status.CompareAndSwap(UNDECIDED, status)
	}

	// phase 2;
	succeeded := cd.status.Load() == SUCCEEDED
	for i := 0; i < cd.n; i++ {
		entry := cd.entries[i].Load()
		if succeeded {
			entry.a2.CompareAndSwap(cd.self, cd.entries[i].Load().n2)
			continue
		}
		entry.a2.CompareAndSwap(cd.self, cd.entries[i].Load().o2)
	}
	return succeeded
}
