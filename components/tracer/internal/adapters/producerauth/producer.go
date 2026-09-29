// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

// Package producerauth resolves platform producers from verified transport
// credentials and authorizes the tenant they act for. A producer is always a
// member of the fixed platform roster; operator configuration only maps
// credentials onto roster names and can never introduce a new producer.
package producerauth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/LerianStudio/midaz/v4/pkg/constant"
)

// ServiceLedger is the ledger's roster name. It is also the integration id
// and the tenant-manager service the ledger acts under.
const ServiceLedger = "ledger"

// Via values record which credential resolved a producer.
const (
	ViaToken = "token"
	ViaCert  = "cert"
)

const (
	maxPlatformProducersBytes = 65536
	maxClientIDBytes          = 256
)

var platformRoster = map[string]struct{}{ServiceLedger: {}}

// Producer is a platform producer resolved from a verified credential.
type Producer struct {
	// Service is the roster name; also the integration id and the
	// tenant-manager service.
	Service string
	// Via is ViaToken or ViaCert, for spans and audit only.
	Via string
}

// Registry maps verified credentials onto roster producers. It is immutable
// after construction and safe for concurrent use.
type Registry struct {
	byClientID map[string]string
	byCertURI  map[string]string
}

type platformProducerEntry struct {
	Service  string `json:"service"`
	ClientID string `json:"clientId"`
	CertURI  string `json:"certUri"`
}

// ParsePlatformProducers decodes TRACER_PLATFORM_PRODUCERS: one non-empty JSON
// array whose entries name a roster service and at least one of clientId or
// certUri. Client ids and certificate URIs must each be unique; several
// entries may map rotating credentials onto the same service. Every violation
// wraps constant.ErrContextPolicyUnavailable and must refuse boot.
func ParsePlatformProducers(raw string) (*Registry, error) {
	if len(raw) == 0 || len(raw) > maxPlatformProducersBytes {
		return nil, fmt.Errorf("%w: TRACER_PLATFORM_PRODUCERS must contain 1 to %d bytes of JSON",
			constant.ErrContextPolicyUnavailable, maxPlatformProducersBytes)
	}

	var entries []platformProducerEntry

	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&entries); err != nil {
		return nil, fmt.Errorf("%w: decode TRACER_PLATFORM_PRODUCERS: %w", constant.ErrContextPolicyUnavailable, err)
	}

	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("%w: TRACER_PLATFORM_PRODUCERS must contain one JSON array", constant.ErrContextPolicyUnavailable)
	}

	return newRegistry(entries)
}

// ByClientID resolves the producer mapped to an access token's authorized
// party. An unmapped client id reports false.
func (r *Registry) ByClientID(clientID string) (Producer, bool) {
	if r == nil || clientID == "" {
		return Producer{}, false
	}

	service, ok := r.byClientID[clientID]
	if !ok {
		return Producer{}, false
	}

	return Producer{Service: service, Via: ViaToken}, true
}

// HasClientIDMappings reports whether any producer can be resolved from an
// access token's authorized party.
func (r *Registry) HasClientIDMappings() bool {
	return r != nil && len(r.byClientID) > 0
}

// HasCertificateMappings reports whether any producer can be resolved from a
// client certificate.
func (r *Registry) HasCertificateMappings() bool {
	return r != nil && len(r.byCertURI) > 0
}

// InRoster reports whether service is a platform producer.
func InRoster(service string) bool {
	_, ok := platformRoster[service]

	return ok
}

// Credentials returns, each sorted, the client ids and certificate URIs mapped
// onto service. It exposes configuration for offline checks only; request
// authentication goes through ByClientID and ByTLS.
func (r *Registry) Credentials(service string) (clientIDs, certURIs []string) {
	if r == nil {
		return nil, nil
	}

	for clientID, mapped := range r.byClientID {
		if mapped == service {
			clientIDs = append(clientIDs, clientID)
		}
	}

	for certURI, mapped := range r.byCertURI {
		if mapped == service {
			certURIs = append(certURIs, certURI)
		}
	}

	slices.Sort(clientIDs)
	slices.Sort(certURIs)

	return clientIDs, certURIs
}

func newRegistry(entries []platformProducerEntry) (*Registry, error) {
	if len(entries) == 0 {
		return nil, fmt.Errorf("%w: TRACER_PLATFORM_PRODUCERS must list at least one producer", constant.ErrContextPolicyUnavailable)
	}

	registry := &Registry{
		byClientID: make(map[string]string, len(entries)),
		byCertURI:  make(map[string]string, len(entries)),
	}

	for i, entry := range entries {
		if _, ok := platformRoster[entry.Service]; !ok {
			return nil, fmt.Errorf("%w: TRACER_PLATFORM_PRODUCERS[%d].service is not a platform producer",
				constant.ErrContextPolicyUnavailable, i)
		}

		if entry.ClientID == "" && entry.CertURI == "" {
			return nil, fmt.Errorf("%w: TRACER_PLATFORM_PRODUCERS[%d] must set clientId or certUri",
				constant.ErrContextPolicyUnavailable, i)
		}

		if entry.ClientID != "" {
			if err := registry.addClientID(i, entry); err != nil {
				return nil, err
			}
		}

		if entry.CertURI != "" {
			if err := registry.addCertURI(i, entry); err != nil {
				return nil, err
			}
		}
	}

	return registry, nil
}

func (r *Registry) addClientID(index int, entry platformProducerEntry) error {
	if !validClientID(entry.ClientID) {
		return fmt.Errorf("%w: TRACER_PLATFORM_PRODUCERS[%d].clientId is malformed", constant.ErrContextPolicyUnavailable, index)
	}

	if _, exists := r.byClientID[entry.ClientID]; exists {
		return fmt.Errorf("%w: TRACER_PLATFORM_PRODUCERS[%d].clientId is duplicated", constant.ErrContextPolicyUnavailable, index)
	}

	r.byClientID[entry.ClientID] = entry.Service

	return nil
}

func (r *Registry) addCertURI(index int, entry platformProducerEntry) error {
	if !validURI(entry.CertURI) {
		return fmt.Errorf("%w: TRACER_PLATFORM_PRODUCERS[%d].certUri is malformed", constant.ErrContextPolicyUnavailable, index)
	}

	if _, exists := r.byCertURI[entry.CertURI]; exists {
		return fmt.Errorf("%w: TRACER_PLATFORM_PRODUCERS[%d].certUri is duplicated", constant.ErrContextPolicyUnavailable, index)
	}

	r.byCertURI[entry.CertURI] = entry.Service

	return nil
}

func validClientID(value string) bool {
	return len(value) <= maxClientIDBytes && utf8.ValidString(value) &&
		strings.TrimSpace(value) == value && !strings.ContainsFunc(value, unicode.IsControl)
}

// validURI accepts only an absolute, canonical URI with a host and no user
// info, query, fragment, wildcard, or whitespace, so a certificate can match
// it only by exact string equality.
func validURI(value string) bool {
	uri, err := url.Parse(value)

	return err == nil && uri.IsAbs() && uri.Host != "" && uri.User == nil && uri.Opaque == "" &&
		uri.RawQuery == "" && !uri.ForceQuery && uri.Fragment == "" && !strings.ContainsAny(value, "* \t\r\n") && uri.String() == value
}
