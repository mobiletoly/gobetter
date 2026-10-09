// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

package structgen

// Signatures returns bootstrap declarations with inert named-result bodies.
// They are type-checking inputs only and must never be published or executed.
// Import finalization and source/build headers belong to the source owner.
func Signatures(plans []Plan) (string, error) {
	return declarations(plans, false)
}
