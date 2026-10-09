// Copyright 2025 Toly Pochkin
// SPDX-License-Identifier: Apache-2.0

// Package config decodes strict schema-v1 JSON and binds records to a supplied
// declaration inventory. It does not discover files, load Go packages, or write
// module/source files. The invocation owner supplies module and package scope;
// the source owner supplies active targets and inactive identities.
package config
