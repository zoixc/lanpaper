# JSON storage scale benchmark — GitHub hosted runner

> **Note:** the raw results below were measured on the former full-file JSON
> backend. SQLite is now the default. Its per-change cost is in the last section.
> The thresholds in the interpretation section describe the JSON backend and
> stay valid for the memory held in RAM, which SQLite does not reduce.

Generated on 2026-10-10 with Go stable on `ubuntu-latest`; three iterations per case.

## Interpretation and supported range

The JSON store remains comfortably supported through **10,000 links** on
hardware comparable to the hosted runner: durable full-file persistence was
about 27 ms and list construction about 1.8 ms, although list responses allocate
roughly 3 MB. At 50,000 records the same operations remain functional, but each
durable mutation rewrites the complete dataset (about 132 ms and 48 MB allocated)
and a list allocates about 15 MB. Concurrent traffic, slower persistent volumes
and richer history/playlist records increase those costs.

Operational policy:

- **0–10k records:** supported JSON-store range.
- **10k–50k records:** transitional range; monitor API p95 latency, process RSS,
  metadata-file size and storage fsync latency.
- Begin the SQLite migration before **10k records**, or earlier when metadata
  mutations exceed **250 ms p95**, `wallpapers.json` exceeds **25 MB**, or peak
  memory is constrained below roughly four times the metadata file size.
- **Above 50k records:** not supported by the JSON backend for production; use
  the SQLite backend introduced by the following roadmap work.

These are baseline microbenchmarks, not a throughput promise. Each result uses
three iterations and an empty-media metadata shape. `BenchmarkStoreScale`
measures map copy plus JSON serialization without disk variability;
`BenchmarkAtomicWriteScale` includes temporary-file write, fsync, rename and
directory fsync.

## Raw results

```text
goos: linux
goarch: amd64
pkg: lanpaper/storage
cpu: AMD EPYC 7763 64-Core Processor                
BenchmarkStoreScale/records=1000/list-4         	       3	     93885 ns/op	  298941 B/op	    1002 allocs/op
BenchmarkStoreScale/records=1000/create-4       	       3	    874689 ns/op	  295949 B/op	    1124 allocs/op
BenchmarkStoreScale/records=1000/update-4       	       3	    787452 ns/op	  112365 B/op	    1009 allocs/op
BenchmarkStoreScale/records=1000/rename-4       	       3	    794251 ns/op	  112381 B/op	    1010 allocs/op
BenchmarkStoreScale/records=1000/delete-4       	       3	    779300 ns/op	  112061 B/op	    1007 allocs/op
BenchmarkStoreScale/records=10000/list-4        	       3	   1826577 ns/op	 2989245 B/op	   10002 allocs/op
BenchmarkStoreScale/records=10000/create-4      	       3	  10299155 ns/op	 6591634 B/op	   10054 allocs/op
BenchmarkStoreScale/records=10000/update-4      	       3	  10345782 ns/op	 9388165 B/op	   10061 allocs/op
BenchmarkStoreScale/records=10000/rename-4      	       3	   9322430 ns/op	 3795496 B/op	   10047 allocs/op
BenchmarkStoreScale/records=10000/delete-4      	       3	  10067204 ns/op	 6591629 B/op	   10052 allocs/op
BenchmarkStoreScale/records=50000/list-4        	       3	  12007933 ns/op	14935229 B/op	   50002 allocs/op
BenchmarkStoreScale/records=50000/create-4      	       3	  47355829 ns/op	15740045 B/op	   50142 allocs/op
BenchmarkStoreScale/records=50000/update-4      	       3	  43287704 ns/op	 4555293 B/op	   50133 allocs/op
BenchmarkStoreScale/records=50000/rename-4      	       3	  45999350 ns/op	15740338 B/op	   50144 allocs/op
BenchmarkStoreScale/records=50000/delete-4      	       3	  46371477 ns/op	15740010 B/op	   50140 allocs/op
BenchmarkAtomicWriteScale/records=1000-4        	       3	   4985522 ns/op	  896808 B/op	    1026 allocs/op
BenchmarkAtomicWriteScale/records=10000-4       	       3	  27433520 ns/op	12840789 B/op	   10039 allocs/op
BenchmarkAtomicWriteScale/records=50000-4       	       3	 131687423 ns/op	47687624 B/op	   50031 allocs/op
PASS
ok  	lanpaper/storage	1.301s
```


## Single change: SQLite against the former JSON rewrite

Measured on the same machine with `BenchmarkSingleUpdate` (`storage/database_bench_test.go`):
one `Store.Update` that toggles the access level of one link, with the library
pre-filled to the stated size. Three iterations, one CPU at 2.6 GHz.

| Records | SQLite (`syncRecords`), three runs | JSON full rewrite, three runs |
|---:|---:|---:|
| 100 | 1.1 ms, 1.2 ms | 1.8 ms, 2.1 ms |
| 1,000 | 1.1 ms, 1.5 ms | 8.0 ms, 8.1 ms |
| 3,000 | 2.0 ms, 3.5 ms | 11.9 ms, 21.2 ms |

Each row is 30 iterations of the benchmark, run on the sandbox machine, which is
noisy: the same configuration varied by up to about 2x between runs. Read the
ratio, not the absolute values.

SQLite cost grows much more slowly because only the changed rows are written.
The residual growth comes from in-memory bookkeeping and the fsync of the WAL.
The JSON figures above are the legacy backend, kept only for stores that were
never loaded.
