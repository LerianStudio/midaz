package utils

import (
	"strings"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
)

// ExtractMongoPortAndParameters handles backward compatibility for MongoDB connection configuration.
// MONGO_PORT=5703/replicaSet=rs0&authSource=admin&directConnection=true
//
// This function extracts the actual port and parameters from such configurations.
// If MONGO_PARAMETERS is already set, it takes precedence over embedded parameters.
//
// Deprecated: This backward compatibility for embedded parameters in MONGO_PORT will be removed
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
