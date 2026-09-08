package pq

import (
	"math"
	"math/bits"
	"math/rand/v2"
	"sync/atomic"
)

type CDNData struct {
	key      string
	value    string
	priority uint32
}

type LNode struct {
	value CDNData
	next  *LNode
}

type CMNode struct {
	list  *LNode
	dirty bool
}

type level struct {
	slots []atomic.Pointer[Word[CMNode]]
}

func newLevel(d uint32) *level {
	lv := &level{
		slots: make([]atomic.Pointer[Word[CMNode]], 1<<d),
	}

	for i := range lv.slots {
		lv.slots[i].Store(&Word[CMNode]{})
	}
	return lv
}

const (
	maxDepth      uint32 = 32
	emptyPriority        = math.MaxInt32
)

type MoundTree struct {
	levels [maxDepth]atomic.Pointer[level]
	depth  atomic.Uint32
}

func NewMoundTree() *MoundTree {
	m := &MoundTree{}
	m.levels[0].Store(newLevel(0))
	m.depth.Store(1)
	return m
}

func (m *MoundTree) Insert(data CDNData) {
	v := data.priority
	for {
		c := m.findInsertPoint(v)
		addr := m.nodeAt(c)
		C := DcssRead(addr)

		if priority(C) < v {
			continue
		}

		C2 := &Word[CMNode]{
			value: CMNode{
				list:  &LNode{value: data, next: C.value.list},
				dirty: C.value.dirty,
			},
		}

		switch c {
		case 1:
			if addr.CompareAndSwap(C, C2) {
				return
			}
		default:
			paddr := m.nodeAt(c / 2)
			P := DcssRead(paddr)
			if priority(P) > v {
				continue
			}

			dcss := NewDcssDescriptor(paddr, P, addr, C, C2)
			if dcss.Dcss() {
				return
			}
		}
	}
}

func (m *MoundTree) findInsertPoint(v uint32) uint32 {
	for {
		d := m.depth.Load()
		for range maxDepth {
			leaf := m.randomLeaf()
			if priority(DcssRead(m.nodeAt(leaf))) >= v {
				return m.binarySearch(leaf, v)
			}
		}

		m.grow(d)
	}
}

func (m *MoundTree) moundify(n uint32) {
	N := DcssRead(m.nodeAt(n))
	d := m.depth.Load()

	if !N.value.dirty || (n >= (1<<(d-1)) && n < (1<<d)) {
		return
	}

	l := DcssRead(m.nodeAt(n * 2))
	r := DcssRead(m.nodeAt(n*2 + 1))

	if !l.value.dirty || !r.value.dirty {
		return
	}

	m.moundify(2 * n)
	m.moundify(2*n + 1)

	if priority(l) <= priority(r) && priority(l) <= priority(N) {
		// TODO:DCAS
		m.moundify(2 * n)
		return
	} else if priority(r) < priority(l) && priority(r) < priority(N) { // TODO:DCAS
		m.moundify(2*n + 1)
		return
	}

	// TODO:CAS
}

func (m *MoundTree) binarySearch(leaf, v uint32) uint32 {
	l, r := 0, bits.Len32(leaf)-1
	ans := leaf
	for l <= r {
		mid := (l + r) >> 1
		c := leaf >> (bits.Len32(leaf) - 1 - mid)
		if priority(DcssRead(m.nodeAt(c))) >= v {
			ans = c
			r = mid - 1
		} else {
			l = mid + 1
		}
	}
	return ans
}

func (m *MoundTree) grow(d uint32) {
	if d >= maxDepth {
		return
	}

	m.levels[d].CompareAndSwap(nil, newLevel(d))
	m.depth.CompareAndSwap(d, d+1)
}

func (m *MoundTree) nodeAt(c uint32) *atomic.Pointer[Word[CMNode]] {
	d := uint32(bits.Len32(c) - 1)
	lv := m.levels[d].Load()
	if lv == nil {
		return nil
	}

	return &lv.slots[c-(1<<d)]
}

func (m *MoundTree) randomLeaf() uint32 {
	d := m.depth.Load()
	base := uint32(1) << (d - 1)

	return base + rand.Uint32N(base)
}
