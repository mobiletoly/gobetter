# Internal planning contract

`source.Parse(filename, bytes)` returns a declaration inventory with physical
positions and original AST identities. It diagnoses malformed, misplaced, and
legacy directives without changing source bytes. A field directive does not
select its containing type; that check waits until configuration and invocation
selection are known.

`config.Decode(filename, bytes)` validates strict schema-v1 JSON, including
duplicate keys, exact property spelling, option syntax, and explicit false values.
`config.Bind` resolves package selectors relative to the configuration file and
validates active target/field identities against caller-supplied inventories.
Out-of-scope records do not select targets. Known inactive identities do not
select plans or validate inactive field semantics. `source.Load` discovers those
identities and qualifies real module/build contexts.

`structgen.MakePlans` consumes parent-before-child targets, bound configuration
records, and an `Invocation` containing only explicitly supplied CLI settings.
It applies defaults, annotations, external records, then invocation overrides.
Output sets replace lower-precedence sets; field settings merge by presence.
Effective field getter requests survive type-level output-set replacement, including
`builder` selection and an explicit CLI `-derive=string`. An annotated or JSON
field getter is independent of the output list; JSON `"getter": false` clears an
annotated request. Private fields require explicit getter requests, and `name=`
provides the same effective setter/getter name. Getter-only selection still needs
at least one requested private-field getter.
Selection combines annotations, configuration, automatic selection, and nested
builders whose parent builder includes their field. User aliases are excluded
from automatic selection and rejected on explicit request.

Each plan owns one effective canonical field slice in source order. `FieldsFor`
derives independent builder/equality/string/getter views by applying each output's
filters without changing declaration order. There is no `Order` model or order
setting. Directives reject either old `order` value as an invalid struct option;
strict JSON rejects `types[].order` as an unknown property; CLI rejects `-order`
as an unknown flag. Getter requests do not alter builder participation. Optional
fields stay in the builder view. Each method's stage is the number of participating
required fields before it; skipped builder fields do not affect that count.
Getter-only plans have no constructor setting and no other output views.

AST references are borrowed source metadata, not mutable rendering scratch space.
`AllocateNames` claims stable step/constructor symbols and suffixes nested alias
collisions against complete package declarations. Generic binders and locals
avoid package symbols and identifiers used by copied syntax. The source adapter
builds a lexical identity map with Go's parser resolver, then verifies selected
substitutions against `go/types` after loading. `RewriteType` copies syntax through
Go formatting/parsing and applies replacements by original node identity; it
does not mutate source nodes or rename members/tags/locals by spelling.

`Signatures` creates typed bootstrap declarations with inert named-result bodies.
These bytes are never executable output. `source.Load` combines all selected
package signatures in one logical overlay, masks only owned in-scope stale
artifacts, and preserves other sources' sidecars in file mode. It preserves source
import syntax and resolves types, generic identities, and cgo-transformed ordinary
field types. Go type checking validates the emitted import bindings; plans do not
store resolved import objects.
`Result.Validate` replaces the complete signature set with caller-rendered final
bytes in one overlay and checks all selected packages/dependencies before any
publication. Normal `go/packages` loader temporaries receive its standard cleanup;
no stubs are written into source/output locations, and Go commands use readonly
module mode. Publication belongs to `internal/generate`, not the source adapter.

Legacy files outside cleanup scope are retained as Go context without extracting
current directives or generation targets. Inactive legacy files are not input
inventories. Active retained declarations/imports/methods still reserve names and
participate in complete-package validation; retaining them grants no ownership.

`Render` consumes resolved/allocated plans and returns `Rendered` containing
runtime declarations and typed standard-library import requests, sharing the
type and method graph with `Signatures`. Targets
retain checked `types.TypeName` objects, including empty structs and generated
aliases; missing target/field/parameter resolution is an error. A caller owns
package headers, preserved import bindings, import finalization, and complete
final-overlay validation. The renderer has no I/O and does not mutate source ASTs.
Import aliases are allocated against complete package/import symbols, copied
type syntax, generic binders, and emitted locals. The caller preserves source
bindings and combines the requests with its package envelope before finalization.

`String` uses value receivers, source declaration order, original member names,
and standard-library `%#v` formatting. An empty view returns `Type{}` without
an import. `Equal` uses resolved Go types: direct supported comparable values,
`slices.Equal` for supported comparable elements, and `maps.Equal` for supported
comparable values. Pointers/channels compare by identity; nil/empty collections
compare equal. Functions and concrete interfaces (also within compared value
aggregates) are rejected at the field, with `skip-equal` as a correction.
Generic parameters require comparison or a uniform collection shape after
intersecting embedded constraint terms, including mixed unions and exact named
types. Exact terms that cannot implement required methods are removed; tilde
terms remain eligible for named implementations. Structurally empty sets, sets
emptied by these exact-term checks, and unsupported shapes are rejected. A
collection parameter must also satisfy the library's structural constraint:
method filtering cannot make a call valid when Go rejects its original terms.
Empty equality
uses a constant comparison so shadowing the predeclared `true` cannot affect it;
normal interface-instantiation panic and floating-point/NaN semantics remain.
User-defined equality methods are never called and values are not deep-compared.
Comparisons run left-to-right in source order and short-circuit with `&&`, after
`skip-equal` filtering. Migration from name order can change cost and panic
behavior: `Pair[T comparable]` with `Z T` before `A int` compares a slice-valued
`Z` in `Pair[any]` before an unequal `A` could return false.

Builder-wide effective setter names must be unique, including across different
stages. Normalization diagnoses collisions deterministically at the later field,
naming both source fields, the effective setter, and the first field's position,
before bootstrap/type loading. `ValidateMembers` also checks these names for
resolved plans. Optional setters reserve `Build` and `BuildPtr` in every window;
the diagnostic says reserved name even on an earlier stage. Required setters may
use those names on their own nonfinal stages. These checks include final-stage
build-method conflicts. Different builders may reuse setter names.
Direct field/authored-method/generated-method conflicts remain validated before
signature emission. Source method inventories cover active
files and retained out-of-scope artifacts, following local non-generic receiver
alias chains across that inventory and retaining physical method positions.
Replaced outputs are masked. Promoted
members may be shadowed under normal Go rules. Fixed constructors/stages diagnose
package-symbol collisions, including effective imports in retained artifacts;
nested aliases retain deterministic suffix allocation. Explicit/default import
bindings and inventoried dot-import exports reserve names; blank imports do not.
Selected packages allocate names in deterministic dependency order using imports
from ordinary active files and retained active artifacts, excluding masked files.
Disk metadata's cycle-pruned import graph does not determine this order. Their
effective exports combine ordinary declarations, retained artifacts, and planned
replacements, excluding masked declarations. Allocation and signature import
handling share this inventory; selected but unallocated inventories produce an
explicit error, while unselected dependencies keep their disk inventory. Selected
default import names use their source package name even when metadata omits an edge.
Result plan ordering and the single combined bootstrap/type-load boundary remain.

Each stage exposes the next required setter and all optional setters in its
source-position window. Optional setters return that same stage; required setters
advance once. Each optional field emits one method. Leading optionals are on the
constructor's initial stage, interleaved optionals close for the advancing chain
at the next required setter, and trailing optionals share the final stage with
both build methods. With zero required fields (including an empty builder view),
the constructor starts on that final stage. Call order among optionals in one
window is arbitrary; repetition assigns the last value.

Field comments are copied verbatim to setters and requested getters, omitting
Gobetter directives. A required setter's comment adds `Next required setter: <name>`
when another required field follows, using its effective setter name, with no
trailing period and a blank `//` separator after copied comments. Optional setters
have no generated stage hint; constructors gain no status comments. Completion
lists the window's optionals alongside the next required setter, including leading
optionals where no preceding method can carry a hint.

An earlier retained stage can still mutate its optional fields after another
chain advances past that window; later builds observe that shared-root mutation.
Both `Build() T` and `BuildPtr() *T` return shallow snapshots. Reference fields
keep normal Go sharing. Builders are not linear or concurrency-safe, and zero-sized
pointer results have no unique-identity promise. Private-field getters use pointer
receivers and normal Go nil dereference behavior, independently of builders.

The `internal/generate` owner assembles/import-finalizes these declarations,
validates complete overlays, and owns artifact comparison/publication. The
`internal/cli` adapter exposes generate/check/version with explicit settings.
The executable uses these owners; superseded legacy helpers have been removed.
No public generator library or Gro adapter is introduced.
