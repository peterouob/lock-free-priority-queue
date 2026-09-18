package pq

import (
	"math"
	"math/bits"
	"math/rand/v2"
	"runtime"
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
	node := &LNode{value: data}
	for {
		c := m.findInsertPoint(v)
		addr := m.nodeAt(c)
		C := dcssRead(addr)

		if priority(C) < v {
			continue
		}
		node.next = C.value.list
		C2 := &Word[CMNode]{value: CMNode{list: node, dirty: C.value.dirty}}

		switch c {
		case 1:
			if addr.CompareAndSwap(C, C2) {
				return
			}
		default:
			paddr := m.nodeAt(c / 2)
			P := dcssRead(paddr)
			if priority(P) > v {
				continue
			}

			dcss := NewDcssDescriptor(paddr, P, addr, C, C2)
			if dcss.Dcss() == C {
				return
			}
		}
	}
}

func (m *MoundTree) findInsertPoint(v uint32) uint32 {
	for {
		d := m.depth.Load()
		for range maxDepth {
			leaf := m.randomLeaf(d)

			addr := m.nodeAt(leaf)
			if addr == nil {
				break
			}

			if priority(dcssRead(addr)) >= v {
				return m.binarySearch(leaf, v)
			}
		}

		m.grow(d)
	}
}

var shardLevel = uint32(bits.Len(uint(runtime.GOMAXPROCS(0) - 1)))

func (m *MoundTree) shardBase() (base, span uint32) {
	d := m.depth.Load()
	L := shardLevel
	if L >= d {
		L = d - 1
	}
	base = uint32(1) << L
	return base, base
}

func (m *MoundTree) tryExtractMin(n uint32) (CDNData, bool) {
	addr := m.nodeAt(n)
	if addr == nil {
		return CDNData{}, false
	}

	N := CasnRead(addr)
	if N.value.dirty {
		m.moundify(n)
		return CDNData{}, false
	}

	if N.value.list == nil {
		return CDNData{}, false
	}

	newN := &Word[CMNode]{value: CMNode{
		list:  N.value.list.next,
		dirty: true,
	}}

	if !addr.CompareAndSwap(N, newN) {
		return CDNData{}, false
	}

	retval := N.value.list.value
	m.moundify(n)
	return retval, true
}

func (m *MoundTree) extractTwoChoice() (CDNData, bool) {
	base, span := m.shardBase()
	if span <= 1 {
		return m.tryExtractMin(1)
	}

	i := base + rand.Uint32N(span)
	j := base + rand.Uint32N(span)

	if i != j {
		ai, aj := m.nodeAt(i), m.nodeAt(j)
		if ai == nil || aj == nil {
			return CDNData{}, false
		}

		if priority(CasnRead(ai)) > priority(CasnRead(aj)) {
			i = j
		}
	}

	return m.tryExtractMin(i)
}

const maxExtractRetry = 3

func (m *MoundTree) RelaxExtractMin() CDNData {
	if addr := m.nodeAt(1); addr != nil {
		R := CasnRead(addr)
		if !R.value.dirty && R.value.list != nil {
			if v, ok := m.tryExtractMin(1); ok {
				return v
			}
		}
	}

	for range maxExtractRetry {
		if v, ok := m.extractTwoChoice(); ok {
			return v
		}
	}

	return m.ExtractMin()
}

func (m *MoundTree) ExtractMin() CDNData {
	for {
		tree := m.nodeAt(1)
		R := CasnRead(tree)

		if R.value.dirty {
			m.moundify(1)
			continue
		}

		if R.value.list == nil {
			return CDNData{}
		}

		newR := &Word[CMNode]{
			value: CMNode{
				list:  R.value.list.next,
				dirty: true,
			},
		}

		if tree.CompareAndSwap(R, newR) {
			retval := R.value.list.value
			m.moundify(1)
			return retval
		}
	}
}

func (m *MoundTree) moundify(n uint32) {
	for {
		addr := m.nodeAt(n)
		if addr == nil {
			return
		}

		N := CasnRead(m.nodeAt(n))

		if !N.value.dirty {
			return
		}

		d := m.depth.Load()

		if n >= (1 << (d - 1)) {
			newN := &Word[CMNode]{value: CMNode{list: N.value.list, dirty: false}}
			if m.nodeAt(n).CompareAndSwap(N, newN) {
				return
			}
			continue
		}

		treetwon := m.nodeAt(n * 2)
		if treetwon == nil {
			newN := &Word[CMNode]{value: CMNode{list: N.value.list, dirty: false}}
			if m.nodeAt(n).CompareAndSwap(N, newN) {
				return
			}
			continue
		}

		l := CasnRead(treetwon)

		if l.value.dirty {
			n = 2 * n
			continue
		}

		treetwonpone := m.nodeAt(n*2 + 1)
		if treetwonpone == nil {
			newN := &Word[CMNode]{value: CMNode{list: N.value.list, dirty: false}}
			if m.nodeAt(n).CompareAndSwap(N, newN) {
				return
			}
			continue
		}

		r := CasnRead(treetwonpone)

		if r.value.dirty {
			n = 2*n + 1
			continue
		}

		switch {
		case priority(l) <= priority(r) && priority(l) < priority(N):
			newN := Word[CMNode]{value: CMNode{
				list:  l.value.list,
				dirty: false,
			}}

			newL := Word[CMNode]{value: CMNode{
				list:  N.value.list,
				dirty: true,
			}}

			e1 := NewCasnEntry(m.nodeAt(n), N, &newN)
			e2 := NewCasnEntry(m.nodeAt(n*2), l, &newL)

			casn := NewCasnDescriptor(e1, e2)

			if casn.Casn() {
				n = 2 * n
				continue
			}
		case priority(r) < priority(l) && priority(r) < priority(N):
			newN := Word[CMNode]{value: CMNode{
				list:  r.value.list,
				dirty: false,
			}}

			newR := Word[CMNode]{value: CMNode{
				list:  N.value.list,
				dirty: true,
			}}

			e1 := NewCasnEntry(m.nodeAt(n), N, &newN)
			e2 := NewCasnEntry(m.nodeAt(n*2+1), r, &newR)

			casn := NewCasnDescriptor(e1, e2)

			if casn.Casn() {
				n = 2*n + 1
				continue
			}
		default:
			newN := Word[CMNode]{value: CMNode{
				list:  N.value.list,
				dirty: false,
			}}

			tree := m.nodeAt(n)
			if tree.CompareAndSwap(N, &newN) {
				return
			}
		}
	}
}

func (m *MoundTree) binarySearch(leaf, v uint32) uint32 {
	l, r := 0, bits.Len32(leaf)-1
	ans := leaf
	for l <= r {
		mid := (l + r) >> 1
		c := leaf >> (bits.Len32(leaf) - 1 - mid)
		if priority(CasnRead(m.nodeAt(c))) >= v {
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

	if m.levels[d].Load() == nil {
		m.levels[d].CompareAndSwap(nil, newLevel(d))
	}
	m.depth.CompareAndSwap(d, d+1)
}

func (m *MoundTree) nodeAt(c uint32) *atomic.Pointer[Word[CMNode]] {
	d := uint32(bits.Len32(c) - 1)
	if d >= m.depth.Load() {
		return nil
	}

	lv := m.levels[d].Load()
	return &lv.slots[c-(1<<d)]
}

func (m *MoundTree) randomLeaf(d uint32) uint32 {
	base := uint32(1) << (d - 1)

	return base + rand.Uint32N(base)
}
