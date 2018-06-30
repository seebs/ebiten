Initial rough-draft benchmarks

This is intended to hold some sample tests to check ebiten's performance
under common loads. The default for testing.Benchmark is to assume that
you want at least one second of runtime for every operation; this does not
suit ebiten's use case well.

	./bench -test.benchmem -test.benchtime=120ms

This will run aiming for about 2ms per frame of ebiten-user rendering time,
which can still easily drop you to 1-2FPS if the GL calls (which aren't
included) are expensive. There's no obvious way to distinguish between
"time spent actually being idle" and "time spent waiting for GL calls".

