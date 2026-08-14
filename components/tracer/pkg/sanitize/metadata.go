// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package sanitize provides utilities for sanitizing sensitive data in audit records.
//
// # Security Context
//
// The Tracer service persists audit snapshots for SOX/GLBA compliance. These snapshots
// include a Metadata field (map[string]any) that can contain arbitrary client-provided data.
// This creates a risk of persisting PII or sensitive data that could violate GDPR/LGPD
// data minimization requirements.
//
// # Compliance Trade-off
//
// | Requirement | SOX/GLBA | GDPR/LGPD | Resolution |
// |-------------|----------|-----------|------------|
// | Full reconstruction | Required | - | Keep structured fields |
// | Data minimization | - | Required | Sanitize arbitrary metadata |
//
// SOX/GLBA requires reconstruction of decision logic, not every piece of metadata.
// GDPR/LGPD requires data minimization. This package implements selective sanitization
// that preserves audit integrity while minimizing sensitive data exposure.
//
// # Defense in Depth
//
// This sanitization is ONE layer of a defense-in-depth strategy:
//
//  1. Database encryption at rest (PostgreSQL TDE or disk encryption)
//  2. Network encryption in transit (TLS)
//  3. Access control (API authentication, database permissions)
//  4. Audit log access logging
//  5. **This sanitization** - Reduces attack surface if other layers fail
//
// # Usage
//
//	sanitizer := sanitize.NewMetadataSanitizer(sanitize.DefaultSensitivePatterns)
//	cleanMetadata := sanitizer.Sanitize(rawMetadata)
package sanitize

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"sync"
)

// MaskedValue is the replacement value for sensitive data.
// Uses a consistent format that indicates sanitization occurred.
const MaskedValue = "[REDACTED]"

// NonSerializablePrefix is the prefix for non-serializable type descriptors.
// Used to indicate values that cannot be JSON marshaled (channels, functions, etc.).
const NonSerializablePrefix = "[non-serializable: "

// DefaultSensitivePatterns contains regex patterns for commonly sensitive keys.
// These patterns are case-insensitive and match partial key names.
//
// Categories covered:
//   - Authentication: password, token, secret, key, auth, bearer, credential
//   - PII identifiers: ssn, social_security, cpf, cnpj, passport, driver_license
//   - Contact PII: email, phone, mobile, address, zip, postal
//   - Financial: card_number, cvv, cvc, account_number, routing, iban, swift
//   - Personal: name (when suffix), birth, dob, age, gender, nationality
var DefaultSensitivePatterns = []string{
	// Authentication & secrets
	`(?i)password`,
	`(?i)passwd`,
	`(?i)secret`,
	`(?i)token`,
	`(?i)api.?key`,
	`(?i)auth`,
	`(?i)bearer`,
	`(?i)credential`,
	`(?i)private.?key`,
	`(?i)access.?key`,

	// Government IDs
	`(?i)ssn`,
	`(?i)social.?security`,
	`(?i)cpf`,  // Brazilian individual taxpayer ID
	`(?i)cnpj`, // Brazilian company taxpayer ID
	`(?i)passport`,
	`(?i)driver.?license`,
	`(?i)national.?id`,

	// Contact information
	`(?i)email`,
	`(?i)e.?mail`,
	`(?i)phone`,
	`(?i)mobile`,
	`(?i)cell`,
	`(?i)fax`,
	`(?i)address`,
	`(?i)street`,
	`(?i)zip.?code`,
	`(?i)postal`,

	// Financial identifiers
	`(?i)card.?number`,
	`(?i)card.?num`,
	`(?i)cvv`,
	`(?i)cvc`,
	`(?i)account.?number`,
	`(?i)account.?num`,
	`(?i)routing`,
	`(?i)iban`,
	`(?i)swift`,
	`(?i)bic`,
	`(?i)\bpin\b`,

	// Personal information
	`(?i)full.?name`,
	`(?i)first.?name`,
	`(?i)last.?name`,
	`(?i)middle.?name`,
	`(?i)birth.?date`,
	`(?i)date.?of.?birth`,
	`(?i)dob`,
	`(?i)maiden`,
	`(?i)mother.?name`,

	// Health (HIPAA)
	`(?i)health`,
	`(?i)medical`,
	`(?i)diagnosis`,
	`(?i)prescription`,

	// IP and device tracking
	`(?i)ip.?address`,
	`(?i)device.?id`,
	`(?i)mac.?address`,
	`(?i)fingerprint`,
}

// MetadataSanitizer sanitizes sensitive keys in metadata maps.
// It is safe for concurrent use as it holds only compiled regexes.
type MetadataSanitizer struct {
	patterns []*regexp.Regexp
}

// NewMetadataSanitizer creates a new sanitizer with the given patterns.
// Patterns are compiled once at construction time for performance.
// Invalid patterns are silently skipped. This is defensive - DefaultSensitivePatterns are all valid.
func NewMetadataSanitizer(patterns []string) *MetadataSanitizer {
	compiled := make([]*regexp.Regexp, 0, len(patterns))

	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			continue
		}

		compiled = append(compiled, re)
	}

	return &MetadataSanitizer{
		patterns: compiled,
	}
}

// Sanitize creates a sanitized copy of the metadata map.
// Keys matching sensitive patterns have their values replaced with MaskedValue.
// Non-serializable values (channels, functions, complex numbers, cyclic references)
// are replaced with type descriptor strings to prevent JSON marshal failures.
// Nested maps are recursively sanitized.
// The original map is NOT modified - a new map is returned.
//
// Returns nil if input is nil (preserves null vs empty distinction).
func (s *MetadataSanitizer) Sanitize(metadata map[string]any) map[string]any {
	if metadata == nil {
		return nil
	}

	if len(metadata) == 0 {
		return map[string]any{}
	}

	// Track visited pointers for cycle detection
	visited := make(map[uintptr]bool)

	return s.sanitizeWithCycleDetection(metadata, visited)
}

// sanitizeWithCycleDetection performs the actual sanitization with cycle detection.
func (s *MetadataSanitizer) sanitizeWithCycleDetection(metadata map[string]any, visited map[uintptr]bool) map[string]any {
	result := make(map[string]any, len(metadata))

	for key, value := range metadata {
		if s.isSensitiveKey(key) {
			result[key] = MaskedValue
			continue
		}

		// Check for non-serializable types and replace with type descriptor
		if desc, isNonSerializable := nonSerializableDescriptor(value, visited); isNonSerializable {
			result[key] = desc
			continue
		}

		// Recursively sanitize nested maps
		if nestedMap, ok := value.(map[string]any); ok {
			result[key] = s.sanitizeWithCycleDetection(nestedMap, visited)
			continue
		}

		// Recursively sanitize slices containing maps
		if slice, ok := value.([]any); ok {
			result[key] = s.sanitizeSliceWithCycleDetection(slice, visited)
			continue
		}

		// Non-sensitive, non-nested, serializable value - copy as-is
		result[key] = value
	}

	return result
}

// isSensitiveKey checks if a key matches any sensitive pattern.
func (s *MetadataSanitizer) isSensitiveKey(key string) bool {
	for _, pattern := range s.patterns {
		if pattern.MatchString(key) {
			return true
		}
	}

	return false
}

// sanitizeSliceWithCycleDetection recursively sanitizes elements in a slice with cycle detection.
// Maps within slices are sanitized; non-serializable values are replaced with type descriptors.
func (s *MetadataSanitizer) sanitizeSliceWithCycleDetection(slice []any, visited map[uintptr]bool) []any {
	if slice == nil {
		return nil
	}

	result := make([]any, len(slice))

	for i, elem := range slice {
		// Check for non-serializable types first
		if desc, isNonSerializable := nonSerializableDescriptor(elem, visited); isNonSerializable {
			result[i] = desc
			continue
		}

		if nestedMap, ok := elem.(map[string]any); ok {
			result[i] = s.sanitizeWithCycleDetection(nestedMap, visited)
		} else if nestedSlice, ok := elem.([]any); ok {
			result[i] = s.sanitizeSliceWithCycleDetection(nestedSlice, visited)
		} else {
			result[i] = elem
		}
	}

	return result
}

// IsSensitiveKey exposes the key check for testing and external use.
// Useful when you need to check a specific key without sanitizing a full map.
func (s *MetadataSanitizer) IsSensitiveKey(key string) bool {
	return s.isSensitiveKey(key)
}

// SanitizeValue masks a single value if the key is sensitive.
// Returns the original value if key is not sensitive.
// Useful for single-field sanitization without map overhead.
func (s *MetadataSanitizer) SanitizeValue(key string, value any) any {
	if s.isSensitiveKey(key) {
		return MaskedValue
	}

	return value
}

// defaultSanitizer is a package-level sanitizer with default patterns.
// Initialized lazily on first use with sync.Once for thread safety.
var (
	defaultSanitizer     *MetadataSanitizer
	defaultSanitizerOnce sync.Once
)

// Default returns the default sanitizer with DefaultSensitivePatterns.
// The sanitizer is created once and reused for all calls.
// Thread-safe via sync.Once.
func Default() *MetadataSanitizer {
	defaultSanitizerOnce.Do(func() {
		defaultSanitizer = NewMetadataSanitizer(DefaultSensitivePatterns)
	})

	return defaultSanitizer
}

// SanitizeMetadata is a convenience function using the default sanitizer.
// Equivalent to Default().Sanitize(metadata).
func SanitizeMetadata(metadata map[string]any) map[string]any {
	return Default().Sanitize(metadata)
}

// MergePatterns combines multiple pattern lists, removing duplicates.
// Useful for extending DefaultSensitivePatterns with custom patterns.
func MergePatterns(patternLists ...[]string) []string {
	seen := make(map[string]bool)

	var result []string

	for _, patterns := range patternLists {
		for _, pattern := range patterns {
			normalized := strings.TrimSpace(pattern)
			if normalized != "" && !seen[normalized] {
				seen[normalized] = true
				result = append(result, normalized)
			}
		}
	}

	return result
}

// nonSerializableDescriptor checks if a value cannot be JSON marshaled and returns
// a type descriptor string if so. This prevents JSON marshal failures in the repository layer.
//
// Non-serializable types include:
//   - Channels (chan T)
//   - Functions (func(...))
//   - Complex numbers (complex64, complex128)
//   - Unsafe pointers
//   - Cyclic references (detected via visited map)
//
// Returns (descriptor, true) if non-serializable, ("", false) otherwise.
func nonSerializableDescriptor(value any, visited map[uintptr]bool) (string, bool) {
	if value == nil {
		return "", false
	}

	v := reflect.ValueOf(value)

	// Check for fundamentally non-serializable types first
	if desc, found := checkFundamentalNonSerializable(v.Kind()); found {
		return desc, true
	}

	// Handle interface, pointer, and struct types
	return checkCompositeNonSerializable(v, visited)
}

// checkFundamentalNonSerializable checks for types that are fundamentally non-serializable.
// Returns (descriptor, true) if the kind is non-serializable, ("", false) otherwise.
func checkFundamentalNonSerializable(kind reflect.Kind) (string, bool) {
	switch kind {
	case reflect.Chan:
		return fmt.Sprintf("%schan]", NonSerializablePrefix), true
	case reflect.Func:
		return fmt.Sprintf("%sfunc]", NonSerializablePrefix), true
	case reflect.Complex64, reflect.Complex128:
		return fmt.Sprintf("%scomplex]", NonSerializablePrefix), true
	case reflect.UnsafePointer:
		return fmt.Sprintf("%sunsafe.Pointer]", NonSerializablePrefix), true
	default:
		return "", false
	}
}

// checkCompositeNonSerializable checks interface, pointer, struct, map, and slice types for non-serializable content.
func checkCompositeNonSerializable(v reflect.Value, visited map[uintptr]bool) (string, bool) {
	switch v.Kind() {
	case reflect.Interface:
		return checkInterfaceNonSerializable(v, visited)
	case reflect.Pointer:
		return checkPointerNonSerializable(v, visited)
	case reflect.Struct:
		return checkStructNonSerializable(v, visited)
	case reflect.Map:
		return checkMapNonSerializable(v, visited)
	case reflect.Slice:
		return checkSliceNonSerializable(v, visited)
	default:
		return "", false
	}
}

// checkInterfaceNonSerializable unwraps interface types and checks the underlying value.
func checkInterfaceNonSerializable(v reflect.Value, visited map[uintptr]bool) (string, bool) {
	if v.IsNil() {
		return "", false
	}

	elem := v.Elem()
	if elem.IsValid() {
		return nonSerializableDescriptor(elem.Interface(), visited)
	}

	return "", false
}

// checkPointerNonSerializable checks pointer types for cyclic references and non-serializable content.
func checkPointerNonSerializable(v reflect.Value, visited map[uintptr]bool) (string, bool) {
	if v.IsNil() {
		return "", false
	}

	ptr := v.Pointer()
	if visited[ptr] {
		return fmt.Sprintf("%scyclic reference]", NonSerializablePrefix), true
	}

	visited[ptr] = true

	elem := v.Elem()
	if elem.IsValid() {
		return nonSerializableDescriptor(elem.Interface(), visited)
	}

	return "", false
}

// checkStructNonSerializable checks struct fields for non-serializable content.
func checkStructNonSerializable(v reflect.Value, visited map[uintptr]bool) (string, bool) {
	if v.CanAddr() {
		ptr := v.Addr().Pointer()
		if visited[ptr] {
			return fmt.Sprintf("%scyclic reference]", NonSerializablePrefix), true
		}

		visited[ptr] = true
	}

	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)

		if t.Field(i).PkgPath == "" && field.CanInterface() {
			if desc, isNonSerializable := nonSerializableDescriptor(field.Interface(), visited); isNonSerializable {
				return desc, true
			}
		}
	}

	return "", false
}

// checkMapNonSerializable checks map types for cyclic references.
// Marks the map as visited to detect cycles in recursive sanitization calls.
func checkMapNonSerializable(v reflect.Value, visited map[uintptr]bool) (string, bool) {
	if v.IsNil() {
		return "", false
	}

	ptr := v.Pointer()
	if visited[ptr] {
		return fmt.Sprintf("%scyclic reference]", NonSerializablePrefix), true
	}

	// Mark as visited to detect cycles in recursive calls
	visited[ptr] = true

	return "", false
}

// checkSliceNonSerializable checks slice types for cyclic references.
// Marks the slice as visited to detect cycles in recursive sanitization calls.
func checkSliceNonSerializable(v reflect.Value, visited map[uintptr]bool) (string, bool) {
	if v.IsNil() {
		return "", false
	}

	ptr := v.Pointer()
	if visited[ptr] {
		return fmt.Sprintf("%scyclic reference]", NonSerializablePrefix), true
	}

	// Mark as visited to detect cycles in recursive calls
	visited[ptr] = true

	return "", false
}

// IsNonSerializable checks if a value cannot be JSON marshaled.
// This is a convenience function for testing and external use.
func IsNonSerializable(value any) bool {
	visited := make(map[uintptr]bool)
	_, isNonSerializable := nonSerializableDescriptor(value, visited)

	return isNonSerializable
}
