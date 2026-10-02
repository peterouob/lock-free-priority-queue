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
			o1 := dcssRead(ctrl)
			o2 := dcssRead(data)
			d := NewDcssDescriptor(ctrl, o1, data, o2, newWord(99))

			data.Store(&d.selfWord)
			d.status.Store(tc.status)

			d.Complete()
			got := dcssRead(data)
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
	o1 := dcssRead(ctrl)
	o2 := dcssRead(data)

	w99 := newWord(99)
	a := NewDcssDescriptor(ctrl, o1, data, o2, w99)
	data.Store(&a.selfWord)
	a.status.Store(SUCCEEDED)

	b := NewDcssDescriptor(ctrl, o1, data, w99, newWord(123))

	done := make(chan struct{})
	go func() {
		b.Dcss()
		close(done)
	}()

	select {
	case <-done:
		assert.Equal(t, 123, dcssRead(data).value)
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

func TestMCASManyWords(t *testing.T) {
	slots := make([]atomic.Pointer[MCASWord[int]], 4)
	olds := make([]*MCASWord[int], len(slots))
	for i := range slots {
		olds[i] = NewWord(i)
		slots[i].Store(olds[i])
	}

	build := func(bad int) []WordDescriptor[int] {
		var ws []WordDescriptor[int]
		for i := len(slots) - 1; i >= 0; i-- {
			old := olds[i]
			if i == bad {
				old = NewWord(-1)
			}
			ws = append(ws, NewWordDescriptor(&slots[i], old, NewWord(i+10)))
		}
		return ws
	}

	d, err := NewMCASDescriptor(build(2)...)
	assert.NoError(t, err)
	assert.False(t, d.MCAS(), "stale old must fail")
	for i := range slots {
		assert.Equal(t, i, Read(&slots[i]).Value(), "failed MCAS changed slot %d", i)
	}

	d, err = NewMCASDescriptor(build(-1)...)
	assert.NoError(t, err)
	assert.True(t, d.MCAS())
	for i := range slots {
		assert.Equal(t, i+10, Read(&slots[i]).Value())
	}
}

func TestCasnManyWords(t *testing.T) {
	slots := make([]atomic.Pointer[Word[int]], 4)
	olds := make([]*Word[int], len(slots))
	for i := range slots {
		olds[i] = newWord(i)
		slots[i].Store(olds[i])
	}

	build := func(bad int) []CasnEntry[int] {
		var es []CasnEntry[int]
		for i := range slots {
			old := olds[i]
			if i == bad {
				old = newWord(-1)
			}
			es = append(es, NewCasnEntry(&slots[i], old, newWord(i+10)))
		}
		return es
	}

	assert.False(t, NewCasnDescriptor(build(2)...).Casn(), "stale old must fail")
	for i := range slots {
		assert.Equal(t, i, CasnRead(&slots[i]).value, "failed CASN changed slot %d", i)
	}

	assert.True(t, NewCasnDescriptor(build(-1)...).Casn())
	for i := range slots {
		assert.Equal(t, i+10, CasnRead(&slots[i]).value)
	}
}
