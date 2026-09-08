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

func (d *DcssDescriptor[V]) Dcss() bool {
	for {
		r := d.a2.Load()
		if r.desc != nil {
			r.desc.Complete()
			continue
		}
		if r != d.o2 {
			return false
		}
		if d.a2.CompareAndSwap(d.o2, d.self) {
			d.Complete()
			return d.status.Load() == SUCCEEDED
		}
	}
}

func (d *DcssDescriptor[V]) Complete() {
	s := d.status.Load()

	if s != UNDECIDED {
		return
	}

	decision := SUCCEEDED

	if DcssRead(d.a1) != d.o1 {
		decision = FAILED
	}

	d.status.CompareAndSwap(UNDECIDED, decision)
	s = d.status.Load()

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
