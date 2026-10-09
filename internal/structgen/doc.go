// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

// Package structgen owns the normalized, internal generation contract.
// Adapters retain source AST identities and produce typed, presence-aware
// settings. Planning resolves selection and precedence without loading packages
// or writing files. Allocation and AST type formatting prepare signature-only
// declarations; source loading verifies their bindings against resolved Go
// objects. Render emits builders/getters, type-aware equality, and string
// declarations with deliberate standard-library import bindings. Package
// envelopes/import finalization/publication have separate owners. No generator
// library is exported outside internal.
package structgen
