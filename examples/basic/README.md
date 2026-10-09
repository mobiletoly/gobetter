# basic

Required source-order steps, optional setter windows, private DOB getters, shallow value/pointer
snapshots, and opt-in equality/string. `WindowedPerson` shows leading `Note`, interleaved `Comment`/
`Alias`, and trailing `Suffix` windows. The initial completion menu offers `Note` and `FirstName`;
after `FirstName`, it offers `Comment`, `Alias`, and `DOB`. Advancing with `DOB` closes that window
for the chain, while a retained earlier stage still mutates the shared root. `Cache` has no setter
and stays zero. Stream retains the inline-interface builder behavior coverage.

From the repository root:

```sh
go run ./examples/basic
go test ./examples/basic
go generate ./examples/basic
```

Expected output:

```text
Ada 1815-12-10 optional
Ada initial late end
```

From this directory, run `go run ../../cmd/gobetter check .` with the same settings as its
package-wide generate directive. Generated sidecars are included in the candidate and intended for
source control.

Benchmark from the root with
`go test -run '^$' -bench 'Benchmark(Direct|Builder)' -benchmem -count=5 ./examples/basic`. Each
value pair assigns to the same value sink; each pointer pair assigns a completed copy to the same
escaping pointer sink. Retained cases create the mutable literal/builder root before the loop and
take fresh snapshots inside it. Large cases copy the same fixed-array payload. Every benchmark uses
B.Loop; timings and allocations are measurements, with no assumed zero-overhead or shared-root
result contract.
