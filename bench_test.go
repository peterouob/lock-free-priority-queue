package pq

import (
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"strconv"
	"strings"
	"sync"
	"testing"
)

type priorityQueue interface {
	Insert(CDNData)
	ExtractMin() CDNData
}

type queueFactory struct {
	name string
	make func(capacity int) priorityQueue
}

var queueFactories = []queueFactory{
	{name: "LockFreeMound", make: func(int) priorityQueue { return NewMoundTree() }},
	{name: "MutexHeap", make: func(capacity int) priorityQueue { return NewLockPQ(capacity) }},
}

type workload int

const (
	workloadInsert workload = iota
	workloadExtractMin
	workloadMixed50
)

const (
	benchSeedA         uint64 = 0x9E3779B97F4A7C15
	benchSeedB         uint64 = 0xBF58476D1CE4E5B9
	benchDefaultOps           = 100000
	benchProfileDirEnv        = "PQ_PROFILE_DIR"
	benchOpsEnv               = "PQ_BENCH_OPS"
)

var (
	benchDataMu    sync.Mutex
	benchDataCache []CDNData
)

func benchData(n int) []CDNData {
	benchDataMu.Lock()
	defer benchDataMu.Unlock()

	if len(benchDataCache) < n {
		r := rand.New(rand.NewPCG(benchSeedA, benchSeedB))
		cache := make([]CDNData, n)
		for i := range cache {
			cache[i] = CDNData{
				key:      "key-" + strconv.Itoa(i),
				value:    "value-" + strconv.Itoa(i),
				priority: r.Uint32N(1 << 30),
			}
		}
		benchDataCache = cache
	}

	return benchDataCache[:n]
}

func totalOps(b *testing.B) int {
	raw := os.Getenv(benchOpsEnv)
	if raw == "" {
		return benchDefaultOps
	}

	n, err := strconv.Atoi(raw)
	if err != nil {
		b.Fatalf("invalid %s=%q: %v", benchOpsEnv, raw, err)
	}
	if n <= 0 {
		b.Fatalf("invalid %s=%d: must be positive", benchOpsEnv, n)
	}

	return n
}

func workerCounts() []int {
	counts := []int{1, 2, 4, 8}
	if p := runtime.GOMAXPROCS(0); p > 8 {
		counts = append(counts, p)
	}

	return counts
}

func profilePath(b *testing.B, dir, suffix string) string {
	name := strings.NewReplacer("/", "_", " ", "_", "#", "_").Replace(b.Name())

	return filepath.Join(dir, name+suffix)
}

func createProfileFile(b *testing.B, path string) *os.File {
	f, err := os.Create(path)
	if err != nil {
		b.Fatalf("create %s: %v", path, err)
	}

	return f
}

func writeLookupProfile(b *testing.B, name, path string) {
	p := pprof.Lookup(name)
	if p == nil {
		b.Fatalf("profile %q not found", name)
	}

	f := createProfileFile(b, path)
	defer func() {
		_ = f.Close()
	}()

	if err := p.WriteTo(f, 0); err != nil {
		b.Fatalf("write %s: %v", path, err)
	}
}

func startProfiles(b *testing.B) func() {
	dir := os.Getenv(benchProfileDirEnv)
	if dir == "" {
		return func() {}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		b.Fatalf("create profile dir %s: %v", dir, err)
	}

	runtime.SetMutexProfileFraction(1)
	runtime.SetBlockProfileRate(1)

	cpuPath := profilePath(b, dir, ".cpu.pprof")
	tracePath := profilePath(b, dir, ".trace.out")

	cpuFile := createProfileFile(b, cpuPath)
	traceFile := createProfileFile(b, tracePath)

	if err := pprof.StartCPUProfile(cpuFile); err != nil {
		b.Fatalf("start cpu profile: %v", err)
	}
	if err := trace.Start(traceFile); err != nil {
		b.Fatalf("start trace: %v", err)
	}

	b.Logf("profiling enabled: %s", dir)

	return func() {
		trace.Stop()
		pprof.StopCPUProfile()

		if err := traceFile.Close(); err != nil {
			b.Fatalf("close %s: %v", tracePath, err)
		}
		if err := cpuFile.Close(); err != nil {
			b.Fatalf("close %s: %v", cpuPath, err)
		}

		runtime.GC()

		writeLookupProfile(b, "heap", profilePath(b, dir, ".heap.pprof"))
		writeLookupProfile(b, "mutex", profilePath(b, dir, ".mutex.pprof"))
		writeLookupProfile(b, "block", profilePath(b, dir, ".block.pprof"))
		writeLookupProfile(b, "goroutine", profilePath(b, dir, ".goroutine.pprof"))

		runtime.SetBlockProfileRate(0)
		runtime.SetMutexProfileFraction(0)

		b.Logf("wrote cpu/heap/mutex/block/goroutine profiles and execution trace for %s", b.Name())
	}
}

func prefaultQueue(q priorityQueue, data []CDNData) {
	for i := range data {
		q.Insert(data[i])
	}
	for range data {
		q.ExtractMin()
	}
}

func fillQueue(q priorityQueue, data []CDNData) {
	for i := range data {
		q.Insert(data[i])
	}
}

func settleHeap() {
	runtime.GC()
	runtime.GC()
}

func runWorkers(q priorityQueue, data []CDNData, workers, perWorker int, w workload) {
	var wg sync.WaitGroup

	for i := range workers {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			chunk := data[id*perWorker : (id+1)*perWorker]

			switch w {
			case workloadInsert:
				for k := range chunk {
					q.Insert(chunk[k])
				}
			case workloadExtractMin:
				for range chunk {
					q.ExtractMin()
				}
			case workloadMixed50:
				for k := range chunk {
					if k%2 == 0 {
						q.Insert(chunk[k])
					} else {
						q.ExtractMin()
					}
				}
			}
		}(i)
	}

	wg.Wait()
}

func runWorkload(b *testing.B, f queueFactory, workers int, w workload) {
	stopProfiles := startProfiles(b)
	defer stopProfiles()

	perWorker := totalOps(b) / workers
	if perWorker == 0 {
		b.Fatalf("total ops %d is smaller than worker count %d", totalOps(b), workers)
	}

	total := workers * perWorker
	data := benchData(total)

	warm := f.make(total)
	prefaultQueue(warm, data)
	settleHeap()

	b.ReportAllocs()
	b.ResetTimer()

	for range b.N {
		b.StopTimer()
		q := f.make(total)
		prefaultQueue(q, data)
		if w != workloadInsert {
			fillQueue(q, data)
		}
		settleHeap()
		b.StartTimer()

		runWorkers(q, data, workers, perWorker, w)
	}

	b.StopTimer()

	elapsed := b.Elapsed().Seconds()
	if elapsed <= 0 {
		b.Fatalf("non-positive elapsed time %v", b.Elapsed())
	}

	ops := float64(b.N) * float64(total)
	b.ReportMetric(ops/elapsed, "ops/s")
	b.ReportMetric(float64(total), "ops/iter")
}

func runMatrix(b *testing.B, w workload) {
	b.Logf("GOMAXPROCS=%d NumCPU=%d totalOps=%d", runtime.GOMAXPROCS(0), runtime.NumCPU(), totalOps(b))

	for _, f := range queueFactories {
		b.Run(f.name, func(b *testing.B) {
			for _, workers := range workerCounts() {
				b.Run("workers="+strconv.Itoa(workers), func(b *testing.B) {
					runWorkload(b, f, workers, w)
				})
			}
		})
	}
}

func BenchmarkInsert(b *testing.B) {
	runMatrix(b, workloadInsert)
}

func BenchmarkExtractMin(b *testing.B) {
	runMatrix(b, workloadExtractMin)
}

func BenchmarkMixed50(b *testing.B) {
	runMatrix(b, workloadMixed50)
}
