# Summary
- We used benchmarks to compare lock-free (NCAS / MCAS mound) and lock-based (single global mutex heap) priority queues.
- Insert: lock-free scales with workers and beats the mutex heap at ≥4 workers (NCAS) / 8 workers (MCAS); the mutex heap wins at low concurrency.
- ExtractMin / ExtractRelax: the mutex heap is ~6–12× faster at every worker count; the mound's root is a serialization point and each extract allocates ~40 objects.
- MCAS vs NCAS: MCAS is faster for extract-heavy workloads at ≥2 workers, but ~1.8× slower on Insert. Fewer CAS is not the main factor — reads that chase lingering descriptors and GC pressure dominate.

# Reference
[mound algo array base pq](https://www.researchgate.net/publication/262397153_Mounds_Array-Based_Concurrent_Priority_Queues)
- when we use golang to implement, we don't need to use counter to avoid an ABA problem on reuse momory because the GC
  - use the pointer with atomic instead can avoid a reuse memory ABA problem, but we should sacrifice more memory

[DCSS algorithm](https://timharris.uk/papers/2002-disc.pdf)

[MCAS algorithm](https://arxiv.org/abs/2008.02527)

# Performance with mutex priority queue

- from the data can know the insert performance on mound-lock-fre-pq will be better than mutex pq when the woker is more and more
- but the extract min/max then control the mutex granularity on mutex can be better than mound pq
  - and the extract relaxation (get the lock min/ max, not the absolute min/max) as the same 
  
**maybe my code got wrong if somebody knows, please tell me thanks**

## Environment

| Item | Value |
|---|---|
| CPU | Apple M1 Pro |
| GOMAXPROCS | 8 |
| NumCPU | 8 |
| totalOps | 100000 |

## BenchmarkInsert

### LockFreeNCASMound

| Workers | iters | ns/op | ms/op | ops/s | B/op | MB/op | allocs/op |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 1 | 62 | 19,176,987 | 19.177 | 5,214,584 | 16,570,893 | 15.80 | 314,270 |
| 2 | 100 | 11,629,200 | 11.629 | 8,599,043 | 16,853,612 | 16.07 | 321,324 |
| 4 | 121 | 9,810,366 | 9.810 | 10,193,299 | 16,968,330 | 16.18 | 324,182 |
| 8 | 139 | 8,538,692 | 8.539 | 11,711,395 | 16,941,930 | 16.16 | 323,492 |

### LockFreeMCASMound

| Workers | iters | ns/op | ms/op | ops/s | B/op | MB/op | allocs/op |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 1 | 34 | 33,638,017 | 33.638 | 2,972,827 | 20,586,158 | 19.63 | 318,316 |
| 2 | 66 | 20,230,155 | 20.230 | 4,943,116 | 20,685,726 | 19.73 | 321,385 |
| 4 | 80 | 15,882,493 | 15.882 | 6,296,241 | 20,779,935 | 19.82 | 324,257 |
| 8 | 92 | 15,311,297 | 15.311 | 6,531,126 | 20,764,645 | 19.80 | 323,676 |

### MutexHeap

| Workers | iters | ns/op | ms/op | ops/s | B/op | MB/op | allocs/op |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 1 | 300 | 4,001,782 | 4.002 | 24,988,867 | 4,800,123 | 4.58 | 100,003 |
| 2 | 127 | 9,340,508 | 9.341 | 10,706,056 | 4,800,278 | 4.58 | 100,005 |
| 4 | 84 | 14,361,939 | 14.362 | 6,962,848 | 4,800,617 | 4.58 | 100,010 |
| 8 | 68 | 17,197,453 | 17.197 | 5,814,814 | 4,801,366 | 4.58 | 100,020 |

## BenchmarkExtractMin

### LockFreeNCASMound

| Workers | iters | ns/op | ms/op | ops/s | B/op | MB/op | allocs/op |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 1 | 4 | 296,872,573 | 296.873 | 336,845 | 402,669,960 | 384.02 | 3,915,032 |
| 2 | 4 | 251,660,990 | 251.661 | 397,360 | 406,315,988 | 387.49 | 3,950,322 |
| 4 | 6 | 188,557,806 | 188.558 | 530,341 | 430,767,464 | 410.81 | 4,187,848 |
| 8 | 5 | 241,940,392 | 241.940 | 413,325 | 490,358,828 | 467.64 | 4,787,491 |

### LockFreeMCASMound

| Workers | iters | ns/op | ms/op | ops/s | B/op | MB/op | allocs/op |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 1 | 4 | 304,846,833 | 304.847 | 328,034 | 248,488,400 | 236.98 | 4,117,415 |
| 2 | 5 | 236,442,342 | 236.442 | 422,936 | 250,626,222 | 239.02 | 4,151,471 |
| 4 | 6 | 173,434,680 | 173.435 | 576,586 | 265,172,342 | 252.89 | 4,389,918 |
| 8 | 6 | 205,699,750 | 205.700 | 486,145 | 290,324,117 | 276.87 | 4,790,574 |

### MutexHeap

| Workers | iters | ns/op | ms/op | ops/s | B/op | MB/op | allocs/op |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 1 | 52 | 23,417,102 | 23.417 | 4,270,383 | 4,800,128 | 4.58 | 100,003 |
| 2 | 32 | 38,401,708 | 38.402 | 2,604,051 | 4,800,224 | 4.58 | 100,005 |
| 4 | 33 | 38,867,510 | 38.868 | 2,572,843 | 4,800,461 | 4.58 | 100,009 |
| 8 | 27 | 38,466,799 | 38.467 | 2,599,644 | 4,801,233 | 4.58 | 100,020 |

## BenchmarkMixed50

### LockFreeNCASMound

| Workers | iters | ns/op | ms/op | ops/s | B/op | MB/op | allocs/op |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 1 | 9 | 129,555,634 | 129.556 | 771,869 | 162,322,994 | 154.80 | 1,668,979 |
| 2 | 12 | 107,304,653 | 107.305 | 931,926 | 163,851,356 | 156.26 | 1,680,092 |
| 4 | 13 | 83,922,641 | 83.923 | 1,191,574 | 173,278,724 | 165.25 | 1,774,265 |
| 8 | 10 | 109,590,100 | 109.590 | 912,491 | 205,782,249 | 196.25 | 2,105,803 |

### LockFreeMCASMound

| Workers | iters | ns/op |   ms/op | ops/s | B/op | MB/op | allocs/op |
|---:|---:|---:|--------:|---:|---:|---:|---:|
| 1 | 8 | 133,462,729 | 133.463 | 749,273 | 108,479,004 | 103.45 | 1,769,528 |
| 2 | 12 | 97,091,275 |  97.091 | 1,029,959 | 109,731,570 | 104.65 | 1,788,966 |
| 4 | 14 | 77,227,411 |  77.227 | 1,294,877 | 115,603,227 | 110.25 | 1,881,200 |
| 8 | 14 | 81,310,943 |  81.311 | 1,229,847 | 134,007,576 | 127.80 | 2,169,661 |

### MutexHeap

| Workers | iters | ns/op | ms/op | ops/s | B/op | MB/op | allocs/op |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 1 | 76 | 13,771,182 | 13.771 | 7,261,541 | 9,813,628 | 9.36 | 100,004 |
| 2 | 52 | 22,897,986 | 22.898 | 4,367,196 | 9,813,736 | 9.36 | 100,006 |
| 4 | 50 | 26,283,368 | 26.283 | 3,804,687 | 9,814,014 | 9.36 | 100,010 |
| 8 | 34 | 32,217,357 | 32.217 | 3,103,917 | 9,814,819 | 9.36 | 100,022 |

## BenchmarkExtractRelax

### LockFreeNCASMound

| Workers | iters | ns/op | ms/op | ops/s | B/op | MB/op | allocs/op |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 1 | 3 | 460,415,875 | 460.416 | 217,195 | 403,806,520 | 385.10 | 3,925,688 |
| 2 | 4 | 261,229,021 | 261.229 | 382,806 | 308,938,752 | 294.63 | 3,036,487 |
| 4 | 8 | 126,748,505 | 126.749 | 788,964 | 308,062,544 | 293.79 | 3,029,141 |
| 8 | 8 | 139,490,182 | 139.490 | 716,896 | 312,778,306 | 298.29 | 3,074,170 |

### LockFreeMCASMound

| Workers | iters | ns/op | ms/op | ops/s | B/op | MB/op | allocs/op |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 1 | 4 | 304,572,302 | 304.572 | 328,329 | 249,183,128 | 237.64 | 4,129,257 |
| 2 | 7 | 169,889,179 | 169.889 | 588,619 | 194,982,816 | 185.95 | 3,204,989 |
| 4 | 9 | 132,943,551 | 132.944 | 752,199 | 196,103,694 | 187.02 | 3,223,000 |
| 8 | 10 | 123,136,025 | 123.136 | 812,110 | 198,623,091 | 189.42 | 3,264,570 |

### MutexHeap

| Workers | iters | ns/op | ms/op | ops/s | B/op | MB/op | allocs/op |
|---:|---:|---:|---:|---:|---:|---:|---:|
| 1 | 54 | 23,080,110 | 23.080 | 4,332,735 | 4,800,120 | 4.58 | 100,003 |
| 2 | 36 | 35,467,338 | 35.467 | 2,819,496 | 4,800,224 | 4.58 | 100,005 |
| 4 | 34 | 34,331,248 | 34.331 | 2,912,798 | 4,800,432 | 4.58 | 100,009 |
| 8 | 32 | 34,832,099 | 34.832 | 2,870,915 | 4,800,876 | 4.58 | 100,017 |
