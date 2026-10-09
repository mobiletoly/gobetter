# unannotated

A deterministic upstream producer owns models.go. gobetter.json selects its annotation-free model and names the private ID setter/getter. One directive file orders the producer before gobetter.

From the repository root:

```sh
go run ./examples/unannotated
go test ./examples/unannotated
go generate ./examples/unannotated
```

Expected output:

```text
upstream model-1 [1 2] schema
```

From this directory, run `go run ../../cmd/gobetter check -config=gobetter.json .` with the same settings as its package-wide generate directive. Generated sidecars are included in the candidate and intended for source control.
