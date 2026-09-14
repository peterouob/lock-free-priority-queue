package pq

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

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
			ctrl := newSlot(0)
			data := newSlot(42)
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
	ctrl := newSlot(0)
	data := newSlot(42)
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
		assert.Equal(t, 123, DcssRead(data).value)
	case <-time.After(time.Second):
		t.Fatal("owner stopped helper cannot do anything")
	}
}

func TestCasnAllOrNothing(t *testing.T) {
	t.Run("succeeds when every word matches", func(t *testing.T) {
		a, b := newSlot(1), newSlot(2)
		cd := NewCasnDescriptor(
			NewCasnEntry(a, a.Load(), newWord(10)),
			NewCasnEntry(b, b.Load(), newWord(20)),
		)

		assert.True(t, cd.Casn())
		assert.Equal(t, 10, CasnRead(a).value)
		assert.Equal(t, 20, CasnRead(b).value)
	})

	t.Run("leaves every word untouched when one does not match", func(t *testing.T) {
		a, b := newSlot(1), newSlot(2)
		stale := newWord(99)
		cd := NewCasnDescriptor(
			NewCasnEntry(a, a.Load(), newWord(10)),
			NewCasnEntry(b, stale, newWord(20)),
		)

		assert.False(t, cd.Casn())
		assert.Equal(t, 1, CasnRead(a).value)
		assert.Equal(t, 2, CasnRead(b).value)
	})
}

func TestCasnHelpedByConcurrentReader(t *testing.T) {
	a := newSlot(1)
	old := a.Load()
	cd := NewCasnDescriptor(NewCasnEntry(a, old, newWord(10)))

	a.Store(cd.self)

	done := make(chan int, 1)
	go func() { done <- CasnRead(a).value }()

	select {
	case got := <-done:
		assert.Equal(t, 10, got)
	case <-time.After(time.Second):
		t.Fatal("reader could not help the stalled CASN descriptor")
	}
}
