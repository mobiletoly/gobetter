# Invocation and artifacts

`Run` loads a selected source set, assembles runtime declarations with the qualified
bootstrap import envelope and explicit standard-library bindings, validates all
final artifacts in one overlay, and compares complete bytes. It requires a Go
1.26 target module and never edits module declarations or authored inputs.

`Check` reports missing, changed, and obsolete owned artifacts without publication.
`Generate` preserves identical files and timestamps, atomically replaces each
changed file through a same-directory temporary, then removes obsolete owned
outputs. Existing inactive-context and file-mode out-of-scope outputs are retained.
Metadata/filename ownership and symlinks are checked before replacement/removal.
No multi-file transaction is promised: filesystem failures return completed paths
and identify the failure so another invocation can converge.

The source adapter rejects emitted pseudo-C signatures before bootstrap import
completion and checked synthesized cgo references before final assembly. Ordinary
Go cgo boundaries remain supported; cgo output retains its implicit constraint.
Native cgo, consumer/tool registration, and package directives are qualified by
isolated tests, using normal Go caches and temporary modules.

The command and published output identify version `0.20.0`. Runnable examples
use this owner through package-wide directives. Repository adoption and author
qualification do not grant owner acceptance or commit/release authority.
