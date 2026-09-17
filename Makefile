test:
	go test -run=XXX -bench=. -benchtime=3s -cpu=1,2,4,8 .
test_ops:
	PQ_PROFILE_DIR=./prof PQ_BENCH_OPS=8192 go test -run=XXX -bench='BenchmarkMixed50/LockFreeMound/workers=8' -benchtime=5x -cpu=8 .
pprof:
	go tool pprof -http=: './prof/BenchmarkMixed50_LockFreeMound_workers=8.cpu.pprof'
trace:
	go tool trace './prof/BenchmarkMixed50_LockFreeMound_workers=8.trace.out'
