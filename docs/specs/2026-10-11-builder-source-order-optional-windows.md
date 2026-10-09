# Source-order builders with optional setter windows

Status: Implementation accepted

Owner spec acceptance (2026-10-11): The owner accepted this spec for implementation and waived another independent spec rereview. The earlier independent review remains NEEDS REVISION; this acceptance does not describe that review as a PASS. Implementation and validation are authorized; commit, push, tagging, publication, and release are not authorized.

Owner implementation acceptance (2026-10-11): After an independent read-only implementation review reported no actionable findings and READY FOR OWNER DECISION, the owner accepted the reviewed implementation. The accepted candidate is the uncommitted worktree on `master` at HEAD `e27b192a7309d932191ce1984591757784dcc812`, identified by review manifest SHA-256 `954bd8ae99627eea94af6b772d39bcff00538c68025e10c0621a816430945705`. Its 36 candidate file hashes, status, diff hashes, retained evidence, and preservation identities were reverified before adding this acceptance record.

The independent implementation review inspected source, coverage, documentation, all five example sidecars, and preservation evidence. Tests, builds, generation, freshness checks, and `git diff --check` were not independently run; the reported passes remain author reports, and raw execution logs were not retained. This records implementation acceptance only. Commit, push, tagging, publication, and release remain unauthorized. The earlier spec review remains NEEDS REVISION.

## Goal

Make a builder's available setters match the order of fields in its source struct. A required field advances the builder to the next stage. An optional field can be set or omitted at its position in that sequence, without moving it to the end or creating a separate branch of builder types.

This is a breaking change targeted at the v0.20 line. This document specifies the change; it does not authorize a release.

## Field order and options

The declaration order of participating struct fields is the only field order. Remove the `order` option entirely: neither `order=source` nor `order=name` is accepted in a struct directive, `types[].order` in JSON configuration, or the `-order` CLI flag. Remove the `Order` type and order settings from planner and invocation models. Existing source field order remains the order for builder setters, getter emission, `Equal` comparisons, and `String` formatting. Each output still applies its own existing field filters before traversing that order. `name=` changes the generated method name, not the field's position.

Source order makes the declaration itself explain builder progression and removes a second ordering rule from annotations, configuration, CLI, and generated outputs. This is a deliberate compatibility trade-off, not a requirement of the optional-window algorithm: that algorithm could also run over name order. Users of upstream-generated source may be unable to rearrange fields; in this design, they can rename or skip setters but cannot reorder them through Gobetter. Getter emission order changes without changing which getters callers can use, while `String` text and `Equal` evaluation can change observably.

Keep `optional` and `skip-builder` as distinct field options:

- An ordinary participating builder field is required and has a setter that advances the stage.
- An `optional` participating field has a setter that returns the same stage. Callers may omit it or call it repeatedly; the last assignment wins. Its setter is available only at its source position, until the next required setter is called.
- A `skip-builder` field has no builder setter and does not advance a stage. It keeps its existing zero-value behavior and independent participation in getters, `Equal`, and `String`.

The existing `optional` plus `skip-builder` conflict remains an error. Field directives and JSON field options otherwise retain their current syntax and override behavior.

Field-level `getter` is an independent request, including when the type directive selects `builder` or a CLI `-derive` replaces the type-level output list. `derive` replaces only type-level outputs; it does not clear effective field getter requests. For example, `//gobetter:struct builder` with `value string //gobetter:field getter` emits both a builder and `Value()`, while `-derive=string` emits `String()` and `Value()` but no builder. A JSON field override of `"getter": false` clears an annotated request. `getters` remains the way to select a target for getters without another type-level output, and each private field still needs an explicit getter request. `name=` supplies the same effective name to the field's setter and requested getter; it does not rename the source field.

Keep the existing attachment grammar: a `//gobetter:struct` comment belongs to the named type's attached leading comment group, without an intervening blank line; it need not be the last comment in that group. An inline struct directive after a type is invalid. A `//gobetter:field` comment may be in a field's leading group or inline after a single field. Options may be reordered and spaced around commas and `=`, but duplicate keys across stacked or mixed leading/inline directives are invalid. A grouped type declaration cannot receive one struct directive for its members. An annotated grouped field such as `A, B int //gobetter:field optional` is invalid; without a field directive, grouped names participate left-to-right and JSON may configure them individually. A struct directive on a direct or pointer anonymous-struct field is a nested-builder request, not an inline directive after `struct { ... }`.

For example, `A` and `B` below are separate required builder fields in that order, while `C` is optional. The nested directive requests a builder for `Config.Database`:

```go
//gobetter:struct builder
type Config struct {
    A, B int
    C string //gobetter:field optional

    //gobetter:struct builder
    Database struct{ Host string }
}
```

By contrast, placing `//gobetter:struct builder` before `type (A struct{}; B struct{})` is invalid: the directive cannot select both declarations in a grouped type block.

## Builder stage contract

Walk builder-participating fields in source order. Let stage `i` be the state after `i` required setters have been called. For each field, assign its method to the stage determined by the number of required fields before it:

- An optional setter is a self-loop on stage `i`.
- A required setter advances from stage `i` to stage `i + 1`.
- `Build()` and `BuildPtr()` exist only on the stage after all required setters.

Thus an optional setter appears once in the generated API, on one receiver type. Several optional fields between the same pair of required fields are all available on that stage, in any call order, and each can be repeated. Calling the next required setter closes that optional window for the advancing chain. Optional fields before the first required field are available from the constructor's initial stage. Optional fields after the last required field share the final stage with `Build()` and `BuildPtr()`. If there are no required fields, the constructor returns that final stage, where all optional setters and both build methods are available. A builder with no participating fields likewise starts on the final stage.

For example:

```go
//gobetter:struct builder
type Person struct {
    // First name of the person
    FirstName string
    // Middle name of the person
    //gobetter:field optional
    MiddleName string
    // Optional comment
    //gobetter:field optional
    Comment string
    // Optional alias
    //gobetter:field optional
    Alias string
    // Date of birth
    //gobetter:field name=DOB
    dob time.Time
}
```

After `NewPersonBuilder().FirstName("Ada").`, completion offers `MiddleName`, `Comment`, `Alias`, and `DOB`. Any optional setters may be called in any order, omitted, or repeated before `DOB`. After `.DOB(date).`, this builder offers `Build` and `BuildPtr`; it no longer offers those three optional setters. The `name=DOB` override is the method name used at that required step.

For a declaration beginning with `Note string //gobetter:field optional` followed by `ID string`, completion immediately after its constructor offers both `Note` and `ID`. Calling `ID` advances past `Note`; calling `Note` stays on the initial stage.

If `MiddleName`, `Comment`, and `Alias` instead follow `dob` in the struct, they appear on the final stage alongside `Build` and `BuildPtr`. Reusing an earlier stage value may still mutate the builder's shared root, as it does today; builders do not enforce linear use. `Build` and `BuildPtr` retain their current shallow-snapshot behavior. For example, these calls remain valid even though the advancing chain has passed `Comment`:

```go
window := NewPersonBuilder().FirstName("Ada")
done := window.DOB(date)
window.Comment("late")
person := done.Build() // person.Comment is "late"
```

## Method names and diagnostics

Validate generated builder methods before bootstrap/type loading. All builder-participating fields must have distinct effective setter names (including `name=` overrides), even when their setters occupy different stages. A collision diagnostic identifies both source fields and the effective name; callers should never need to interpret `.Value(...).Value(...)` as two different fields. Preserve `Build` and `BuildPtr` as reserved names for optional setters in every window, including earlier windows where those build methods are unavailable. Report a reserved-name error there rather than claiming a final-stage collision. Final-stage optional setters must not conflict with `Build` or `BuildPtr`. Keep existing getter, `Equal`, `String`, constructor, stage-type, and authored-member collision checks; different builders may use the same setter name.

Rejected `order` input must produce a clear error at its input boundary: an invalid struct option for either directive form, an unknown JSON property under the strict schema, and an unknown CLI flag. Do not silently accept or ignore the old option. Preserve stage-local validation against build methods and source-positioned diagnostics for all generated method conflicts.

## Generated comments

Continue copying field comments verbatim to its generated setter and requested getter. Required setter comments continue to include the generated `Next required setter: <name>` hint when another required setter follows. Derive `<name>` from the next required field's effective setter name, including `name=`. If source comments precede the hint, separate the hint with a blank `//` line. The hint has no trailing period. Optional setters do not advance the stage and do not receive this hint solely because another required setter follows.

For the example above, the `FirstName` setter's generated comment is:

```go
// First name of the person
//
// Next required setter: DOB
```

Each optional setter keeps its own field comment without a generated stage hint. The generated comment contract stays minimal: optional methods and the constructor gain no new status prose. Developer documentation must explain that completion presents the available optional setters alongside the next required setter, including on the initial stage when no preceding required method can carry a hint.

## Migration and repository work

An optional field between required fields can no longer be set after the later required setter. For example, callers must change `NewPersonBuilder().FirstName("Ada").DOB(date).Comment("x").Build()` to `NewPersonBuilder().FirstName("Ada").Comment("x").DOB(date).Build()` when `Comment` appears between `FirstName` and `dob` in source. Callers using `order=name` may see a different required-setter sequence; `String` field order and generated text may also change. Remove `order` from configuration, directives, and commands instead of providing a compatibility alias.

Existing builders that reuse one effective setter name for different fields on different stages must assign distinct `name=` values or skip one of those setters. This breaks chains such as `.Value("first").Value("second")` deliberately, so each method name continues to identify one field throughout a builder.

`Equal` evaluates field comparisons left-to-right with short-circuiting `&&`. Removing `order=name` changes which comparisons run first and may change comparison cost or whether a call panics. For example, a previously name-ordered `Pair[T comparable]` declared with `Z T` before `A int` checks `A` first. With `Pair[any]{Z: []int{1}, A: 1}` versus `Pair[any]{Z: []int{1}, A: 2}`, that old `Equal` returns false before comparing `Z`; source-order `Equal` compares `Z` first and panics because its interface value holds a slice. This runtime consequence is part of the breaking-change decision. See the Go specification on [short-circuit evaluation](https://go.dev/ref/spec#Logical_operators) and [comparable type arguments](https://go.dev/ref/spec#Satisfying_a_type_constraint).

The implementation should update the planner's field views, renderer and generated signatures, builder-wide setter-name validation, directive/config/CLI parsers, user and internal documentation, examples, fixtures, and tests. Regenerate owned sidecars from their authored inputs. Preserve unrelated generated output and source edits. `README.md` should show optional windows, the leading-window completion menu, and the retained-stage shared-root example; retain its clarified `derive`/field-getter rule; explain annotation attachment/grouping with examples; describe the `Equal` migration risk; and remove all `order` instructions except migration guidance. `internal/structgen/README.md` should describe the new stage and getter contracts.

## Acceptance criteria

1. Compile-time examples accept optional calls before the next required setter, in either order and with repetition; they reject those calls after advancing past that window, an early `Build`/`BuildPtr`, and a required setter called out of order.
2. Optional setters before the first required field, after the last required field, and with zero required fields have the documented method availability. `skip-builder` fields do not affect stage count. One optional field generates one setter method, regardless of how many optional fields share its window.
3. Duplicate effective setter names across any two participating fields produce deterministic, source-positioned diagnostics before bootstrap/type loading, including when the fields occupy different stages. Optional `Build`/`BuildPtr` names receive reserved-name diagnostics. The next-required comment hint uses the effective name and keeps its blank separator and no-period format.
4. All generated output traverses participating fields in source order. Removed `order` inputs fail with specific, stable diagnostics through directives, strict JSON config, and CLI; omitted settings work without an order model. An `Equal` runtime case demonstrates source-order short-circuiting and the possible changed panic behavior for `Pair[any]` with a slice-valued `Z`.
5. Field getter requests survive type-level `derive` replacement through annotation, JSON, and CLI routes. A JSON `"getter": false` override clears an annotated getter request. Getter-only generation and the existing private-field/name rules remain covered.
6. Documentation, examples, parity/runtime fixtures, and generated sidecars reflect the new contract, including leading optionals and retained-stage mutation. The repository test suite passes, and the annotated examples plus the config-driven unannotated example pass generation freshness checks.

For validation after implementation, run `GOTOOLCHAIN=go1.26.9 GOWORK=off go test ./...`; check annotated examples with `go run ./cmd/gobetter check ./examples/basic ./examples/embedded ./examples/generics ./examples/nested`; and, from `examples/unannotated`, check with `go run ../../cmd/gobetter check -config=gobetter.json .`. Inspect generated diffs and run `git diff --check`. These are implementation checks, not required to create this spec.
