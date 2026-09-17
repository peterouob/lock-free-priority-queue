package pq

import (
	"reflect"
	"strconv"
	"testing"
)

func TestLockPQOrder(t *testing.T) {
	q := NewLockPQ(8)

	in := []uint32{10, 20, 30, 40, 50, 5}
	for i, p := range in {
		q.Insert(CDNData{priority: p, value: strconv.Itoa(100 + i)})
	}

	var got []uint32
	for range in {
		got = append(got, q.ExtractMin().priority)
	}

	want := []uint32{5, 10, 20, 30, 40, 50}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("drained %v, want %v", got, want)
	}

	if q.Len() != 0 {
		t.Fatalf("queue length %d after full drain, want 0", q.Len())
	}

	if zero := q.ExtractMin(); zero != (CDNData{}) {
		t.Fatalf("extract from empty queue returned %+v, want zero value", zero)
	}

	t.Logf("drained %v in order", got)
}
