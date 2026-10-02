package pq

import (
	"math/bits"
	"math/rand/v2"
	"sync/atomic"
)

type mWord = MCASWord[CMNode]
type mSlot = atomic.Pointer[mWord]

type mcasLevel struct {
	slots []mSlot
}

func newMCASLevel(d uint32) *mcasLevel {
	lv := &mcasLevel{slots: make([]mSlot, 1<<d)}
	for i := range lv.slots {
		lv.slots[i].Store(NewWord(CMNode{}))
	}
	return lv
}

// MCASMoundTree is NCASMoundTree with every write going through MCAS
// (https://arxiv.org/pdf/2008.02527) instead of CAS/DCSS/CASN.
type MCASMoundTree struct {
	levels [maxDepth]atomic.Pointer[mcasLevel]
	depth  atomic.Uint32
}

func NewMCASMoundTree() *MCASMoundTree {
	m := &MCASMoundTree{}
	m.levels[0].Store(newMCASLevel(0))
	m.depth.Store(1)
	return m
}

func mPriority(w *mWord) uint32 {
	if w == nil || w.Value().list == nil {
		return emptyPriority
	}
	return w.Value().list.value.priority
}

func mcasRun(words ...WordDescriptor[CMNode]) bool {
	d, err := NewMCASDescriptor(words...)
	if err != nil {
		panic(err) // only reachable on programmer error (dup/nil addr)
	}
	return d.MCAS()
}

func cas1(addr *mSlot, old, new *mWord) bool {
	return mcasRun(NewWordDescriptor(addr, old, new))
}

func cas2(a1 *mSlot, o1, n1 *mWord, a2 *mSlot, o2, n2 *mWord) bool {
	return mcasRun(NewWordDescriptor(a1, o1, n1), NewWordDescriptor(a2, o2, n2))
}

// setClean writes N back with dirty=false.
func (m *MCASMoundTree) setClean(addr *mSlot, N *mWord) bool {
	return cas1(addr, N, NewWord(CMNode{list: N.Value().list}))
}

func (m *MCASMoundTree) Insert(data CDNData) {
	v := data.priority
	node := &LNode{value: data}
	for {
		c := m.findInsertPoint(v)
		addr := m.nodeAt(c)
		C := Read(addr)
		if mPriority(C) < v {
			continue
		}
		node.next = C.Value().list
		C2 := NewWord(CMNode{list: node, dirty: C.Value().dirty})

		if c == 1 {
			if cas1(addr, C, C2) {
				return
			}
			continue
		}

		paddr := m.nodeAt(c / 2)
		P := Read(paddr)
		if mPriority(P) > v {
			continue
		}
		// parent P->P acts as the DCSS guard
		if cas2(paddr, P, P, addr, C, C2) {
			return
		}
	}
}

func (m *MCASMoundTree) findInsertPoint(v uint32) uint32 {
	for {
		d := m.depth.Load()
		for range maxDepth {
			leaf := m.randomLeaf(d)
			addr := m.nodeAt(leaf)
			if addr == nil {
				break
			}
			if mPriority(Read(addr)) >= v {
				return m.binarySearch(leaf, v)
			}
		}
		m.grow(d)
	}
}

func (m *MCASMoundTree) shardBase() (base, span uint32) {
	d := m.depth.Load()
	L := shardLevel
	if L >= d {
		L = d - 1
	}
	base = uint32(1) << L
	return base, base
}

func (m *MCASMoundTree) tryExtractMin(n uint32) (CDNData, bool) {
	addr := m.nodeAt(n)
	if addr == nil {
		return CDNData{}, false
	}

	N := Read(addr)
	if N.Value().dirty {
		m.moundify(n)
		return CDNData{}, false
	}
	if N.Value().list == nil {
		return CDNData{}, false
	}

	if !cas1(addr, N, NewWord(CMNode{list: N.Value().list.next, dirty: true})) {
		return CDNData{}, false
	}

	m.moundify(n)
	return N.Value().list.value, true
}

func (m *MCASMoundTree) extractTwoChoice() (CDNData, bool) {
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
		if mPriority(Read(ai)) > mPriority(Read(aj)) {
			i = j
		}
	}

	return m.tryExtractMin(i)
}

func (m *MCASMoundTree) RelaxExtractMin() CDNData {
	if addr := m.nodeAt(1); addr != nil {
		R := Read(addr)
		if !R.Value().dirty && R.Value().list != nil {
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

func (m *MCASMoundTree) ExtractMin() CDNData {
	for {
		root := m.nodeAt(1)
		R := Read(root)

		if R.Value().dirty {
			m.moundify(1)
			continue
		}
		if R.Value().list == nil {
			return CDNData{}
		}

		if cas1(root, R, NewWord(CMNode{list: R.Value().list.next, dirty: true})) {
			m.moundify(1)
			return R.Value().list.value
		}
	}
}

func (m *MCASMoundTree) moundify(n uint32) {
	for {
		addr := m.nodeAt(n)
		if addr == nil {
			return
		}

		N := Read(addr)
		if !N.Value().dirty {
			return
		}

		d := m.depth.Load()
		laddr, raddr := m.nodeAt(2*n), m.nodeAt(2*n+1)
		if n >= (1<<(d-1)) || laddr == nil || raddr == nil {
			if m.setClean(addr, N) {
				return
			}
			continue
		}

		l := Read(laddr)
		if l.Value().dirty {
			n = 2 * n
			continue
		}
		r := Read(raddr)
		if r.Value().dirty {
			n = 2*n + 1
			continue
		}

		switch {
		case mPriority(l) <= mPriority(r) && mPriority(l) < mPriority(N):
			if cas2(addr, N, NewWord(CMNode{list: l.Value().list}),
				laddr, l, NewWord(CMNode{list: N.Value().list, dirty: true})) {
				n = 2 * n
			}
		case mPriority(r) < mPriority(l) && mPriority(r) < mPriority(N):
			if cas2(addr, N, NewWord(CMNode{list: r.Value().list}),
				raddr, r, NewWord(CMNode{list: N.Value().list, dirty: true})) {
				n = 2*n + 1
			}
		default:
			if m.setClean(addr, N) {
				return
			}
		}
	}
}

func (m *MCASMoundTree) binarySearch(leaf, v uint32) uint32 {
	l, r := 0, bits.Len32(leaf)-1
	ans := leaf
	for l <= r {
		mid := (l + r) >> 1
		c := leaf >> (bits.Len32(leaf) - 1 - mid)
		if mPriority(Read(m.nodeAt(c))) >= v {
			ans = c
			r = mid - 1
		} else {
			l = mid + 1
		}
	}
	return ans
}

func (m *MCASMoundTree) grow(d uint32) {
	if d >= maxDepth {
		return
	}
	if m.levels[d].Load() == nil {
		m.levels[d].CompareAndSwap(nil, newMCASLevel(d))
	}
	m.depth.CompareAndSwap(d, d+1)
}

func (m *MCASMoundTree) nodeAt(c uint32) *mSlot {
	d := uint32(bits.Len32(c) - 1)
	if d >= m.depth.Load() {
		return nil
	}
	return &m.levels[d].Load().slots[c-(1<<d)]
}

func (m *MCASMoundTree) randomLeaf(d uint32) uint32 {
	base := uint32(1) << (d - 1)
	return base + rand.Uint32N(base)
}
