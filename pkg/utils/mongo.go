package utils

import (
	"fmt"
	"strings"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
)

// ExtractMongoPortAndParameters handles backward compatibility for MongoDB connection configuration.
// MONGO_PORT=5703/replicaSet=rs0&authSource=admin&directConnection=true
//
// This function extracts the actual port and parameters from such configurations.
// If MONGO_PARAMETERS is already set, it takes precedence over embedded parameters.
//
// DEPRECATED: This backward compatibility for embedded parameters in MONGO_PORT will be removed
// in Midaz 4.0.0. Update environment variables to use the MONGO_PARAMETERS environment variable.
func ExtractMongoPortAndParameters(port, parameters string, logger libLog.Logger) (string, string) {
	// Always sanitize port by stripping any embedded "/..." or "?..." suffix
	actualPort := port
	if idx := strings.IndexAny(port, "/?"); idx != -1 {
		actualPort = port[:idx]
		embeddedParams := port[idx+1:]

		// If parameters are already explicitly set, warn and ignore embedded params
		if parameters != "" {
			logger.Warnf(
				"DEPRECATED: MongoDB parameters embedded in MONGO_PORT detected but ignored "+
					"(MONGO_PARAMETERS takes precedence). Remove embedded parameters from MONGO_PORT. "+
					"Sanitized port=%s, ignored embedded=%s, using explicit=%s",
				actualPort, embeddedParams, parameters,
			)

			return actualPort, parameters
		}

		// Use embedded params (legacy behavior)
		logger.Warnf(
			"DEPRECATED: MongoDB parameters embedded in MONGO_PORT detected. "+
				"Update environment variables to use the MONGO_PARAMETERS environment variable. "+
				"Extracted port=%s, parameters=%s",
			actualPort, embeddedParams,
		)

		return actualPort, embeddedParams
	}

	return actualPort, parameters
}

// BuildMongoConnectionString constructs a properly formatted MongoDB connection string.
// It ensures the correct format: scheme://user:password@host:port/?parameters
//
// The trailing slash before the query string is required by the MongoDB URI specification
// when no database name is provided. This function centralizes connection string building
// to prevent formatting bugs across components.
//
// Parameters:
//   - uri: The MongoDB scheme (e.g., "mongodb", "mongodb+srv")
//   - user: The username for authentication
//   - password: The password for authentication
//   - host: The MongoDB host address
//   - port: The MongoDB port number
//   - parameters: Optional query parameters (e.g., "replicaSet=rs0&authSource=admin")
//   - logger: Optional logger for debugging connection string construction (sensitive data is masked)
//
// Returns the complete connection string ready for use with MongoDB drivers.
func BuildMongoConnectionString(uri, user, password, host, port, parameters string, logger libLog.Logger) string {
	// Build base connection string with trailing slash (required before query params)
	connectionString := fmt.Sprintf("%s://%s:%s@%s:%s/", uri, user, password, host, port)

	// Append parameters if present
	if parameters != "" {
		connectionString += "?" + parameters
	}

	// Log connection string for debugging (mask all credentials)
	if logger != nil {
		maskedConnStr := fmt.Sprintf("%s://<credentials>@%s:%s/", uri, host, port)
		if parameters != "" {
			maskedConnStr += "?" + parameters
		}

		logger.Debugf("MongoDB connection string built: %s", maskedConnStr)
	}

	return connectionString
}
