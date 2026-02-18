// Package openapi provides the embedded OpenAPI specification.
package openapi

import _ "embed"

// Spec is the OpenAPI 3.1 specification (YAML).
//go:embed spec.yaml
var Spec []byte
