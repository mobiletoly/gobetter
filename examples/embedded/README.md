# embedded

Value, pointer, and instantiated generic embedded fields are single required assignments. The Owner pointer remains shared.

From the repository root:

```sh
go run ./examples/embedded
go test ./examples/embedded
go generate ./examples/embedded
```

Expected output:

```text
London Ada 7
```

From this directory, run `go run ../../cmd/gobetter check .` with the same settings as its package-wide generate directive. Generated sidecars are included in the candidate and intended for source control.
