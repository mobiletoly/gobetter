# nested

Direct and pointer anonymous structs receive assignment-compatible aliases. JSON tags survive; the explicit ID name produces an exported setter/getter for _id. TLS is optional.

From the repository root:

```sh
go run ./examples/nested
go test ./examples/nested
go generate ./examples/nested
```

Expected output:

```text
1 localhost postgres true
```

From this directory, run `go run ../../cmd/gobetter check .` with the same settings as its package-wide generate directive. Generated sidecars are included in the candidate and intended for source control.
