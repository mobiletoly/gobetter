// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

// Package source adapts source bytes and attached gobetter directives to the
// internal declaration inventory. Parse is in-memory and accepts upstream-
// generated source without altering it. Load discovers active packages/files,
// inventories inactive declarations and owned artifacts, prepares combined
// signature overlays, and resolves original Go bindings. Result.Validate checks
// a combined final-artifact overlay. Neither operation publishes source/output
// files; normal Go caches and cleaned-up loader temporaries may be used.
package source
