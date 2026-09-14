package pq

import (
	"math/bits"
	"reflect"
	"strconv"
	"sync"
	"testing"
)

func buildTree(vals []int) *MoundTree {
	m := &MoundTree{}
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

func TestBinarySearch(t *testing.T) {
	m := buildTree([]int{0, 1, 3, 5, 7, 9, 11, 13})
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

func checkMoundProperty(t *testing.T, m *MoundTree) {
	d := m.depth.Load()
	for c := uint32(2); c < 1<<d; c++ {
		p, ch := priority(dcssRead(m.nodeAt(c/2))), priority(dcssRead(m.nodeAt(c)))
		if p > ch {
			t.Errorf("violated at %d: parent=%d child=%d", c, p, ch)
		}
	}
}

func TestExtractMinOrder(t *testing.T) {
	m := NewMoundTree()

	in := []uint32{10, 20, 30, 40, 50, 5}
	for i, p := range in {
		m.Insert(CDNData{priority: p, value: strconv.Itoa(100 + i)})
	}

	var got []uint32
	for range in {
		d := m.ExtractMin()
		got = append(got, d.priority)
		checkMoundProperty(t, m)
	}

	want := []uint32{5, 10, 20, 30, 40, 50}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("drained %v, want %v", got, want)
	}
}

func TestInsertConcurrent(t *testing.T) {
	m := NewMoundTree()
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
