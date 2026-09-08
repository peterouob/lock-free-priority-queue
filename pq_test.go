package pq

import (
	"math/bits"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
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
		p, ch := priority(DcssRead(m.nodeAt(c/2))), priority(DcssRead(m.nodeAt(c)))
		if p > ch {
			t.Errorf("violated at %d: parent=%d child=%d", c, p, ch)
		}
	}
}

func TestInsert(t *testing.T) {
	m := NewMoundTree()
	for _, p := range []uint32{10, 20, 30, 40, 50, 5} {
		m.Insert(CDNData{priority: p})
		checkMoundProperty(t, m)
	}
	if got := priority(DcssRead(m.nodeAt(1))); got != 5 {
		t.Errorf("root = %d, want 5", got)
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

func TestCompleteClearDescriptor(t *testing.T) {

	cases := []struct {
		name   string
		status int32
		want   int
	}{
		{"succeeded", SUCCEEDED, 99},
		{"failed", FAILED, 42},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := newSlot[int](0)
			data := newSlot[int](42)
			o1 := DcssRead(ctrl)
			o2 := DcssRead(data)
			d := NewDcssDescriptor(ctrl, o1, data, o2, newWord(99))

			data.Store(d.self)
			d.status.Store(tc.status)

			d.Complete()
			got := DcssRead(data)
			assert.Nil(t, got.desc, "a2 still gave descriptor")

			assert.Equal(t, tc.want, got.value)
		})
	}
}

func newWord[V any](value V) *Word[V] {
	return &Word[V]{
		value: value,
	}
}

func newSlot[V any](v V) *atomic.Pointer[Word[V]] {
	p := new(atomic.Pointer[Word[V]])
	w := newWord(v)
	p.Store(w)
	return p
}

func TestHelperMakesProgressOnStalledDescriptor(t *testing.T) {
	ctrl := newSlot[int](0)
	data := newSlot[int](42)
	o1 := DcssRead(ctrl)
	o2 := DcssRead(data)

	w99 := newWord(99)
	a := NewDcssDescriptor(ctrl, o1, data, o2, w99)
	data.Store(a.self)
	a.status.Store(SUCCEEDED)

	b := NewDcssDescriptor(ctrl, o1, data, w99, newWord(123))

	done := make(chan struct{})
	go func() {
		b.Dcss()
		close(done)
	}()

	select {
	case <-done:
		assert.Equal(t, 123, data.Load().value)
	case <-time.After(time.Second):
		t.Fatal("owner stopped helper cannot do anything")
	}
}
