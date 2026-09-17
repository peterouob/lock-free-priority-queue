### This is the repo using the mound algo implement the priority queue
- and i use this pq on my cdn server project been base to build the cache componment

# Reference
[mound algo array base pq](https://www.researchgate.net/publication/262397153_Mounds_Array-Based_Concurrent_Priority_Queues)

[DCSS algorithm](https://timharris.uk/papers/2002-disc.pdf)

# Performance with mutex pq

## Insert

| workers | Mound ns/op | Mound allocs | MutexHeap ns/op | MutexHeap allocs |
|---------|-------------|--------------|-----------------|------------------|
| 1       | 237.9       | 4.17         | 40.2            | 1.00             |
| 2       | 140.7       | 4.25         | 94.0            | 1.00             |
| 4       | 109.1       | 4.20         | 141.4           | 1.00             |
| 8       | 101.6       | 4.20         | 170.0           | 1.00             |