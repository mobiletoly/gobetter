# generics

Generic builders preserve fixed arrays and the Go 1.26 self-referencing Next constraint. Completed value/pointer results are scalar snapshots.

From the repository root:

```sh
go run ./examples/generics
go test ./examples/generics
go generate ./examples/generics
```

Expected output:

```text
hello [one two] 3
```

From this directory, run `go run ../../cmd/gobetter check .` with the same settings as its package-wide generate directive. Generated sidecars are included in the candidate and intended for source control.
