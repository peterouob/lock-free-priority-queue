package pq

import (
	"fmt"
	"math/bits"
	"math/rand/v2"
	"strconv"
	"sync"
	"testing"
)

type moundImpl struct {
	new   func() MoundTree
	build func(vals []int) MoundTree
	name  string
}

var moundImpls = []moundImpl{
	{name: "NCAS", new: func() MoundTree { return NewMoundTree() }, build: buildNCAS},
	{name: "MCAS", new: func() MoundTree { return NewMCASMoundTree() }, build: buildMCAS},
}

func forEachMound(t *testing.T, f func(t *testing.T, impl moundImpl)) {
	for _, impl := range moundImpls {
		t.Run(impl.name, func(t *testing.T) { f(t, impl) })
	}
}

func buildNCAS(vals []int) MoundTree {
	m := &NCASMoundTree{}
	d := uint32(bits.Len32(uint32(len(vals) - 1)))
	for lv := range d {
		m.levels[lv].Store(newLevel(lv))
	}
	m.depth.Store(d)
	for c := 1; c < len(vals); c++ {
		m.nodeAt(uint32(c)).Store(&Word[CMNode]{
			value: CMNode{list: &LNode{value: CDNData{priority: uint32(vals[c])}}},
		})
	}
	return m
}

func buildMCAS(vals []int) MoundTree {
	m := &MCASMoundTree{}
	d := uint32(bits.Len32(uint32(len(vals) - 1)))
	for lv := range d {
		m.levels[lv].Store(newMCASLevel(lv))
	}
	m.depth.Store(d)
	for c := 1; c < len(vals); c++ {
		m.nodeAt(uint32(c)).Store(NewWord(CMNode{list: &LNode{value: CDNData{priority: uint32(vals[c])}}}))
	}
	return m
}

func TestBinarySearch(t *testing.T) {
	forEachMound(t, testBinarySearch)
}

func testBinarySearch(t *testing.T, impl moundImpl) {
	m := impl.build([]int{0, 1, 3, 5, 7, 9, 11, 13})
	cases := []struct {
		v          int
		leaf, want uint32
	}{
		{6, 7, 7},
		{4, 7, 3},
		{0, 7, 1},
		{13, 7, 7},
	}
	for _, c := range cases {
		if got := m.binarySearch(c.leaf, uint32(c.v)); got != c.want {
			t.Errorf("v=%d leaf=%d: got %d, want %d", c.v, c.leaf, got, c.want)
		}
	}
}

func moundDepth(m MoundTree) uint32 {
	switch m := m.(type) {
	case *NCASMoundTree:
		return m.depth.Load()
	case *MCASMoundTree:
		return m.depth.Load()
	}
	panic(fmt.Sprintf("unknown MoundTree %T", m))
}

func moundNode(m MoundTree, n uint32) (prio uint32, dirty, ok bool) {
	switch m := m.(type) {
	case *NCASMoundTree:
		if addr := m.nodeAt(n); addr != nil {
			w := CasnRead(addr)
			return priority(w), w.value.dirty, true
		}
	case *MCASMoundTree:
		if addr := m.nodeAt(n); addr != nil {
			w := Read(addr)
			return mPriority(w), w.Value().dirty, true
		}
	default:
		panic(fmt.Sprintf("unknown MoundTree %T", m))
	}
	return 0, false, false
}

func checkMoundProperty(t *testing.T, m MoundTree) {
	t.Helper()
	limit := uint32(1) << moundDepth(m)

	for n := uint32(1); n < limit; n++ {
		p, dirty, ok := moundNode(m, n)
		if !ok || dirty {
			continue
		}

		for _, c := range []uint32{2 * n, 2*n + 1} {
			cp, _, ok := moundNode(m, c)
			if !ok {
				continue
			}
			if p > cp {
				t.Errorf("violated at %d: parent=%d child=%d", n, p, cp)
			}
		}
	}
}

func TestExtractMinOrderRandom(t *testing.T) {
	forEachMound(t, testExtractMinOrderRandom)
}

func testExtractMinOrderRandom(t *testing.T, impl moundImpl) {
	for trial := range 100 {
		m := impl.new()
		n := 50 + rand.IntN(200)
		vals := rand.Perm(n)
		for i, v := range vals {
			m.Insert(CDNData{priority: uint32(v), value: strconv.Itoa(i)})
		}
		for i := range n {
			got := m.ExtractMin()
			if got.priority != uint32(i) {
				t.Fatalf("trial %d: pop %d, want %d", trial, got.priority, i)
			}
			checkMoundProperty(t, m)
		}
	}
}

func TestInsertConcurrent(t *testing.T) {
	forEachMound(t, testInsertConcurrent)
}

func testInsertConcurrent(t *testing.T, impl moundImpl) {
	m := impl.new()
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(base int) {
			defer wg.Done()
			for j := range 100 {
				m.Insert(CDNData{priority: uint32(base*100) + uint32(j)})
			}
		}(i)
	}
	wg.Wait()
	checkMoundProperty(t, m)
}

func TestExtractMinConcurrent(t *testing.T) {
	forEachMound(t, testExtractMinConcurrent)
}

func testExtractMinConcurrent(t *testing.T, impl moundImpl) {
	const n = 1000
	m := impl.new()
	for i := range n {
		m.Insert(CDNData{priority: uint32(i), value: strconv.Itoa(i)})
	}

	var mu sync.Mutex
	seen := make(map[string]bool, n)
	ch := make(chan string, n)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range n / 8 {
				d := m.ExtractMin()
				mu.Lock()
				if seen[d.value] {
					t.Errorf("value %s extracted twice", d.value)
				}
				seen[d.value] = true
				ch <- d.value
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	close(ch)

	if len(seen) != n {
		t.Fatalf("extracted %d distinct items, want %d", len(seen), n)
	}
	if d := m.ExtractMin(); d.value != "" {
		t.Fatalf("queue not empty after drain: %+v", d)
	}
}

func TestExtractMinMonotonic(t *testing.T) {
	forEachMound(t, testExtractMinMonotonic)
}

func testExtractMinMonotonic(t *testing.T, impl moundImpl) {
	const n = 1000
	m := impl.new()
	for i := range n {
		m.Insert(CDNData{priority: uint32(rand.Perm(n)[i]), value: strconv.Itoa(i)})
	}

	prev := uint32(0)
	for i := range n {
		d := m.ExtractMin()
		if i > 0 && d.priority < prev {
			t.Fatalf("extract %d: priority %d < previous %d", i, d.priority, prev)
		}
		prev = d.priority
	}
}

func TestStressMixed(t *testing.T) {
	forEachMound(t, testStressMixed)
}

func testStressMixed(t *testing.T, impl moundImpl) {
	for trial := range 30 {
		m := impl.new()
		const W, per = 8, 3000
		var mu sync.Mutex
		got := map[string]int{}
		var wg sync.WaitGroup
		for w := range W {
			wg.Go(func() {
				r := rand.New(rand.NewPCG(uint64(trial), uint64(w)))
				var local []string
				for j := range per {
					id := strconv.Itoa(w*per + j)
					m.Insert(CDNData{value: id, priority: r.Uint32N(5000)})
					if r.IntN(3) > 0 {
						var d CDNData
						if r.IntN(2) == 0 {
							d = m.ExtractMin()
						} else {
							d = m.RelaxExtractMin()
						}
						if d.value != "" {
							local = append(local, d.value)
						}
					}
				}
				mu.Lock()
				for _, v := range local {
					got[v]++
				}
				mu.Unlock()
			})
		}
		wg.Wait()
		checkMoundProperty(t, m)
		prev := uint32(0)
		for {
			d := m.ExtractMin()
			if d.value == "" {
				break
			}
			if d.priority < prev {
				t.Fatalf("trial %d: drain not sorted %d < %d", trial, d.priority, prev)
			}
			prev = d.priority
			got[d.value]++
		}
		for i := range W * per {
			if c := got[strconv.Itoa(i)]; c != 1 {
				t.Fatalf("trial %d: item %d seen %d times", trial, i, c)
			}
		}
	}
}
