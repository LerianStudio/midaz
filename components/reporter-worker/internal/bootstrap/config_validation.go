// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	pkgConstant "github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
)

// Validate checks that all required configuration fields are present.
// Returns a descriptive multi-error message listing all missing fields.
func (c *Config) Validate() error {
	var errs []string

	errs = append(errs, c.requiredFieldErrors()...)
	errs = append(errs, validateTCPPort("RABBITMQ_PORT_AMQP", c.RabbitMQPortAMQP)...)
	errs = append(errs, c.multiTenantValidationErrors()...)
	errs = append(errs, validateWorkerAbsoluteURL("RABBITMQ_HEALTH_CHECK_URL", c.RabbitMQHealthCheckURL, c.EnvName)...)
	errs = c.validateProductionConfig(errs)

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed:\n- %s", strings.Join(errs, "\n- "))
	}

	return nil
}

func (c *Config) requiredFieldErrors() []string {
	required := []struct {
		value string
		name  string
	}{
		{c.RabbitMQHost, "RABBITMQ_HOST"},
		{c.RabbitMQPortAMQP, "RABBITMQ_PORT_AMQP"},
		{c.RabbitMQUser, "RABBITMQ_DEFAULT_USER"},
		{c.RabbitMQPass, "RABBITMQ_DEFAULT_PASS"},
		{c.RabbitMQGenerateReportQueue, "RABBITMQ_GENERATE_REPORT_QUEUE"},
	}

	if !c.MultiTenantEnabled {
		required = append(required,
			struct {
				value string
				name  string
			}{c.MongoDBHost, "MONGO_HOST"},
			struct {
				value string
				name  string
			}{c.MongoDBName, "MONGO_NAME"},
		)
	}

	return missingWorkerRequiredFields(required)
}

func missingWorkerRequiredFields(required []struct {
	value string
	name  string
},
) []string {
	errs := make([]string, 0)

	for _, field := range required {
		if field.value == "" {
			errs = append(errs, field.name+" is required")
		}
	}

	return errs
}

func (c *Config) multiTenantValidationErrors() []string {
	if !c.MultiTenantEnabled {
		return nil
	}

	errs := make([]string, 0)
	if c.MultiTenantURL == "" {
		errs = append(errs, "MULTI_TENANT_URL is required when MULTI_TENANT_ENABLED=true")
	} else {
		errs = append(errs, validateWorkerAbsoluteURL("MULTI_TENANT_URL", c.MultiTenantURL, c.EnvName)...)
	}

	if c.MultiTenantCircuitBreakerThreshold <= 0 {
		errs = append(errs, "MULTI_TENANT_CIRCUIT_BREAKER_THRESHOLD must be > 0 when MULTI_TENANT_ENABLED=true (default: 5)")
	}

	if c.MultiTenantCircuitBreakerThreshold > 0 && c.MultiTenantCircuitBreakerTimeoutSec <= 0 {
		errs = append(errs, "MULTI_TENANT_CIRCUIT_BREAKER_TIMEOUT_SEC must be > 0 when MULTI_TENANT_CIRCUIT_BREAKER_THRESHOLD > 0 (default: 30)")
	}

	if c.RedisHost == "" {
		errs = append(errs, "REDIS_HOST is required when MULTI_TENANT_ENABLED=true (used for tenant discovery cache)")
	}

	if c.MultiTenantServiceAPIKey == "" {
		errs = append(errs, "MULTI_TENANT_SERVICE_API_KEY is required when MULTI_TENANT_ENABLED=true")
	}

	return errs
}

func validateTCPPort(name, value string) []string {
	if value == "" {
		return nil // emptiness is caught by requiredFieldErrors
	}

	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return []string{name + " must be a valid TCP port number (1-65535)"}
	}

	return nil
}

func validateWorkerAbsoluteURL(name, rawURL, envName string) []string {
	if rawURL == "" {
		return nil
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
		return []string{name + " must be a valid absolute URL"}
	}

	scheme := strings.ToLower(parsedURL.Scheme)
	if scheme != "http" && scheme != "https" {
		return []string{name + " must use http or https scheme"}
	}

	if strings.EqualFold(envName, "production") && scheme == "http" {
		return []string{name + " must use HTTPS in production"}
	}

	return nil
}

// validateProductionConfig enforces stricter rules when EnvName is "production".
// Telemetry and real credentials are required in production.
func (c *Config) validateProductionConfig(errs []string) []string {
	if !strings.EqualFold(c.EnvName, "production") {
		return errs
	}

	if !c.EnableTelemetry {
		errs = append(errs, "ENABLE_TELEMETRY must be true in production")
	}

	secrets := []struct {
		value string
		name  string
	}{
		{c.MongoDBPassword, "MONGO_PASSWORD"},
		{c.RabbitMQPass, "RABBITMQ_DEFAULT_PASS"},
		{c.ObjectStorageSecretKey, "OBJECT_STORAGE_SECRET_KEY"},
		{c.CryptoHashSecretKeyPluginCRM, "CRYPTO_HASH_SECRET_KEY_PLUGIN_CRM"},
		{c.CryptoEncryptSecretKeyPluginCRM, "CRYPTO_ENCRYPT_SECRET_KEY_PLUGIN_CRM"},
	}

	for _, s := range secrets {
		switch s.value {
		case "":
			errs = append(errs, s.name+" must not be empty in production")
		case pkgConstant.DefaultPasswordPlaceholder:
			errs = append(errs, s.name+" must not use the default placeholder in production")
		}
	}

	// TLS enforcement is bypassable via ALLOW_INSECURE_TLS (mirrors
	// lib-commons semantics: truthy = bypass, default false = enforce).
	// Non-TLS production checks (telemetry, secrets) are always enforced.
	if !c.AllowInsecureTLS {
		if c.ObjectStorageDisableSSL {
			errs = append(errs, "OBJECT_STORAGE_DISABLE_SSL must be false in production")
		}

		if !usesSecureRabbitMQScheme(c.RabbitURI) {
			errs = append(errs, "RABBITMQ_URI must use AMQPS in production")
		}
	}

	if c.ObjectStorageEndpoint != "" {
		parsedURL, err := url.Parse(c.ObjectStorageEndpoint)
		if err != nil || parsedURL.Scheme == "" || parsedURL.Host == "" {
			errs = append(errs, "OBJECT_STORAGE_ENDPOINT must be a valid absolute URL in production")
		} else if strings.EqualFold(parsedURL.Scheme, "http") {
			errs = append(errs, "OBJECT_STORAGE_ENDPOINT must use HTTPS in production")
		}
	}

	if c.MultiTenantEnabled {
		errs = c.validateProductionMultiTenant(errs)
	}

	return errs
}

func (c *Config) validateProductionMultiTenant(errs []string) []string {
	// TLS-related multi-tenant checks honor the ALLOW_INSECURE_TLS opt-out.
	// The non-TLS REDIS_PASSWORD requirement stays enforced regardless.
	if !c.AllowInsecureTLS {
		if !c.RedisTLS {
			errs = append(errs, "REDIS_TLS must be true in production when MULTI_TENANT_ENABLED=true")
		}

		if c.MultiTenantRedisHost != "" && !c.MultiTenantRedisTLS {
			errs = append(errs, "MULTI_TENANT_REDIS_TLS must be true in production when MULTI_TENANT_REDIS_HOST is configured")
		}
	}

	if !c.RedisUseGCPIAM && c.RedisPassword == "" {
		errs = append(errs, "REDIS_PASSWORD must not be empty in production when MULTI_TENANT_ENABLED=true and REDIS_USE_GCP_IAM=false")
	}

	return errs
}

func usesSecureRabbitMQScheme(rawValue string) bool {
	rawValue = strings.TrimSpace(strings.ToLower(rawValue))
	if rawValue == "" {
		return false
	}

	if strings.Contains(rawValue, "://") {
		parsedURL, err := url.Parse(rawValue)
		if err != nil {
			return false
		}

		rawValue = strings.ToLower(parsedURL.Scheme)
	}

	return rawValue == "amqps"
}
