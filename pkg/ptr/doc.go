package ptr

// Package ptr provides small helper functions for working with pointers to
// basic types. These helpers are primarily used to conveniently construct
// optional fields when building request/response payloads or test fixtures.
//
// The helpers return addresses of values while keeping call sites concise and
// readable, e.g., ptr.StringPtr("value").

