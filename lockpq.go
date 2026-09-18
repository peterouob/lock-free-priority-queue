package pq

import (
	"container/heap"
	"sync"
)

type lockHeap []CDNData

func (h *lockHeap) Len() int { return len(*h) }

func (h *lockHeap) Less(i, j int) bool { return (*h)[i].priority < (*h)[j].priority }

func (h *lockHeap) Swap(i, j int) { (*h)[i], (*h)[j] = (*h)[j], (*h)[i] }

func (h *lockHeap) Push(x any) { *h = append(*h, x.(CDNData)) }

func (h *lockHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

type LockPQ struct {
	mu sync.Mutex
	h  lockHeap
}

func (q *LockPQ) RelaxExtractMin() CDNData {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.h.Len() == 0 {
		return CDNData{}
	}

	return heap.Pop(&q.h).(CDNData)
}

func NewLockPQ(capacity int) *LockPQ {
	return &LockPQ{h: make(lockHeap, 0, capacity)}
}

func (q *LockPQ) Insert(data CDNData) {
	q.mu.Lock()
	heap.Push(&q.h, data)
	q.mu.Unlock()
}

func (q *LockPQ) ExtractMin() CDNData {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.h.Len() == 0 {
		return CDNData{}
	}

	return heap.Pop(&q.h).(CDNData)
}

func (q *LockPQ) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()

	return q.h.Len()
}
