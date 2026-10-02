package pq

import (
	"cmp"
	"slices"
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

	selfWord Word[V]

	status atomic.Int32
}

func (d *DcssDescriptor[C, V]) init(
	a1 *atomic.Pointer[Word[C]], o1 *Word[C],
	a2 *atomic.Pointer[Word[V]], o2 *Word[V],
	n2 *Word[V],
) {
	d.a1, d.o1, d.a2, d.o2, d.n2 = a1, o1, a2, o2, n2
	d.selfWord.desc = d
}

func NewDcssDescriptor[C, V any](
	a1 *atomic.Pointer[Word[C]], o1 *Word[C],
	a2 *atomic.Pointer[Word[V]], o2 *Word[V],
	n2 *Word[V],
) *DcssDescriptor[C, V] {
	d := &DcssDescriptor[C, V]{}
	d.init(a1, o1, a2, o2, n2)
	return d
}

func (d *DcssDescriptor[C, V]) dcssDescriptor() {}

func (d *DcssDescriptor[C, V]) Dcss() *Word[V] {
	self := &d.selfWord
	for {
		r := d.a2.Load()
		if isDesc, ok := r.desc.(dcssDescriptor); ok {
			isDesc.Complete()
			continue
		}
		if r != d.o2 {
			return r
		}
		if d.a2.CompareAndSwap(d.o2, self) {
			d.Complete()
			return d.o2
		}
	}
}

func (d *DcssDescriptor[C, V]) Complete() {
	s := d.status.Load()

	if s == UNDECIDED {
		decision := SUCCEEDED
		if dcssRead(d.a1) != d.o1 {
			decision = FAILED
		}
		d.status.CompareAndSwap(UNDECIDED, decision)
		s = d.status.Load()
	}

	self := &d.selfWord
	if s == SUCCEEDED {
		d.a2.CompareAndSwap(self, d.n2)
		return
	}

	d.a2.CompareAndSwap(self, d.o2)
}

func dcssRead[V any](addr *atomic.Pointer[Word[V]]) *Word[V] {
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

type casnSlot[V any] struct {
	dcss DcssDescriptor[int32, V]
	new  *Word[V]
}

type CasnDescriptor[V any] struct {
	status   atomic.Pointer[Word[int32]]
	selfWord Word[V]
	slots    []casnSlot[V]
	inlines  [2]casnSlot[V]
}

func NewCasnDescriptor[V any](entries ...CasnEntry[V]) *CasnDescriptor[V] {
	switch {
	case len(entries) == 2:
		if uintptr(unsafe.Pointer(entries[1].addr)) > uintptr(unsafe.Pointer(entries[0].addr)) {
			entries[0], entries[1] = entries[1], entries[0]
		}
	case len(entries) > 2:
		slices.SortFunc(entries, func(a, b CasnEntry[V]) int {
			return cmp.Compare(uintptr(unsafe.Pointer(b.addr)), uintptr(unsafe.Pointer(a.addr)))
		})
	}

	c := &CasnDescriptor[V]{}
	if len(entries) <= cap(c.inlines) {
		c.slots = c.inlines[:len(entries)]
	} else {
		c.slots = make([]casnSlot[V], len(entries))
	}
	c.status.Store(CasnUndecided)
	c.selfWord.desc = c

	for i, e := range entries {
		c.slots[i].dcss.init(&c.status, CasnUndecided, e.addr, e.old, &c.selfWord)
		c.slots[i].new = e.new
	}

	return c
}

func (cd *CasnDescriptor[V]) Complete() {}

func (cd *CasnDescriptor[V]) Casn() bool {
	self := &cd.selfWord

	if cd.status.Load() == CasnUndecided {
		status := CasnSucceeded

		for i := 0; i < len(cd.slots) && status == CasnSucceeded; i++ {
			d := &cd.slots[i].dcss

		retry:
			switch val := d.Dcss(); val {
			case d.o2:
			case self:
			default:
				if other, ok := val.desc.(*CasnDescriptor[V]); ok && other != cd {
					other.Casn()
					goto retry
				}
				status = CasnFailed
			}
		}
		cd.status.CompareAndSwap(CasnUndecided, status)
	}

	succeeded := cd.status.Load() == CasnSucceeded
	for i := range cd.slots {
		d := &cd.slots[i].dcss
		if succeeded {
			d.a2.CompareAndSwap(self, cd.slots[i].new)
			continue
		}
		d.a2.CompareAndSwap(self, d.o2)
	}

	return succeeded
}

func CasnRead[V any](addr *atomic.Pointer[Word[V]]) *Word[V] {
	for {
		r := dcssRead(addr)
		isCasn, ok := r.desc.(*CasnDescriptor[V])
		if !ok {
			return r
		}

		isCasn.Casn()
	}
}
