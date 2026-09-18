# Reference
[mound algo array base pq](https://www.researchgate.net/publication/262397153_Mounds_Array-Based_Concurrent_Priority_Queues)
- when we use golang to implement, we don't need to use counter to avoid an ABA problem on reuse momory because the GC
  - use the pointer with atomic instead can avoid a reuse memory ABA problem, but we should sacrifice more memory

[DCSS algorithm](https://timharris.uk/papers/2002-disc.pdf)

# Performance with mutex priority queue

- from the data can know the insert performance on mound-lock-fre-pq will be better than mutex pq when the woker is more and more
- but the extract min/max then control the mutex granularity on mutex can be better than mound pq
  - and the extract relaxation (get the lock min/ max, not the absolute min/max) as the same 
  
**maybe my code got wrong if somebody knows, please tell me thanks**

## Environment

| Item | Value |
|---|---|
| GOMAXPROCS | 8 |
| NumCPU | 8 |
| totalOps | 100000 |

## BenchmarkInsert

| Impl | Workers | iters | ns/op | ms/op | ops/s | B/op | MB/op | allocs/op |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| LockFreeMound | 1 | 64 | 18,964,046 | 18.964 | 5,273,136 | 16,819,297 | 16.04 | 320,481 |
| LockFreeMound | 2 | 91 | 11,985,163 | 11.985 | 8,343,649 | 16,620,364 | 15.85 | 315,499 |
| LockFreeMound | 4 | 123 | 9,811,141 | 9.811 | 10,192,494 | 16,993,647 | 16.21 | 324,825 |
| LockFreeMound | 8 | 139 | 8,580,960 | 8.581 | 11,653,708 | 16,863,427 | 16.08 | 321,549 |
| MutexHeap | 1 | 295 | 3,997,089 | 3.997 | 25,018,205 | 4,800,123 | 4.58 | 100,003 |
| MutexHeap | 2 | 128 | 9,229,024 | 9.229 | 10,835,382 | 4,800,249 | 4.58 | 100,005 |
| MutexHeap | 4 | 80 | 13,755,121 | 13.755 | 7,270,019 | 4,800,642 | 4.58 | 100,010 |
| MutexHeap | 8 | 68 | 16,831,333 | 16.831 | 5,941,300 | 4,801,356 | 4.58 | 100,021 |

## BenchmarkExtractMin

| Impl | Workers | iters | ns/op | ms/op | ops/s | B/op | MB/op | allocs/op |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| LockFreeMound | 1 | 4 | 277,249,573 | 277.250 | 360,686 | 402,813,390 | 384.15 | 3,916,363 |
| LockFreeMound | 2 | 5 | 226,912,542 | 226.913 | 440,698 | 407,925,164 | 389.02 | 3,965,492 |
| LockFreeMound | 4 | 6 | 192,021,889 | 192.022 | 520,774 | 431,575,170 | 411.58 | 4,196,299 |
| LockFreeMound | 8 | 5 | 246,302,242 | 246.302 | 406,005 | 502,975,721 | 479.68 | 4,912,019 |
| MutexHeap | 1 | 50 | 22,259,622 | 22.260 | 4,492,439 | 4,800,120 | 4.58 | 100,003 |
| MutexHeap | 2 | 36 | 34,507,329 | 34.507 | 2,897,935 | 4,800,574 | 4.58 | 100,006 |
| MutexHeap | 4 | 32 | 35,393,151 | 35.393 | 2,825,405 | 4,801,264 | 4.58 | 100,011 |
| MutexHeap | 8 | 30 | 35,386,171 | 35.386 | 2,825,963 | 4,802,890 | 4.58 | 100,024 |

## BenchmarkExtractRelax

| Impl | Workers | iters | ns/op | ms/op | ops/s | B/op | MB/op | allocs/op |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| LockFreeMound | 1 | 4 | 273,518,125 | 273.518 | 365,606 | 401,465,476 | 382.87 | 3,903,738 |
| LockFreeMound | 2 | 6 | 178,734,882 | 178.735 | 559,488 | 307,130,824 | 292.90 | 3,019,595 |
| LockFreeMound | 4 | 8 | 128,663,188 | 128.663 | 777,223 | 307,095,050 | 292.87 | 3,020,040 |
| LockFreeMound | 8 | 9 | 125,514,167 | 125.514 | 796,723 | 317,504,465 | 302.80 | 3,118,946 |
| MutexHeap | 1 | 51 | 22,838,325 | 22.838 | 4,378,605 | 4,800,120 | 4.58 | 100,003 |
| MutexHeap | 2 | 33 | 35,144,279 | 35.144 | 2,845,413 | 4,800,416 | 4.58 | 100,005 |
| MutexHeap | 4 | 32 | 35,720,979 | 35.721 | 2,799,475 | 4,801,074 | 4.58 | 100,010 |
| MutexHeap | 8 | 32 | 36,106,009 | 36.106 | 2,769,622 | 4,801,410 | 4.58 | 100,018 |