package mongo

import (
	"fmt"
	"strings"

	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	"github.com/iancoleman/strcase"
	"go.mongodb.org/mongo-driver/bson"
)

func BuildDocumentToPatch(updateDocument bson.M, fieldsToRemove []string) bson.M {
	flatDocument := bson.M{}
	flattenBSONM(updateDocument, "", flatDocument)

	setMap := bson.M{}
	unsetMap := bson.M{}

	for k, v := range flatDocument {
		if !shouldUnset(k, fieldsToRemove) {
			setMap[k] = v
		}
	}

	for _, v := range fieldsToRemove {
		if strings.HasPrefix(v, "metadata.") {
			unsetMap[v] = ""
		} else {
			unsetMap[strcase.ToSnakeWithIgnore(v, ".")] = v
		}
	}

	update := bson.M{}
	if len(setMap) > 0 {
		update["$set"] = setMap
	}

	if len(unsetMap) > 0 {
		update["$unset"] = unsetMap
	}

	return update
}

// flattenBSONM recursively flattens a nested BSON map.
func flattenBSONM(m bson.M, prefix string, flat bson.M) {
	for k, v := range m {
		var key string
		if prefix == "" {
			key = k
		} else {
			key = prefix + "." + k
		}

		if sub, ok := v.(bson.M); ok {
			flattenBSONM(sub, key, flat)
		} else {
			flat[key] = v
		}
	}
}

// shouldUnset Checks if the key should be "unset" (removed) based on the fieldsToRemove array.
// If the field to be removed is "addresses.primary", we remove "addresses.primary" as well as "addresses.primary.*"
func shouldUnset(key string, fieldsToRemove []string) bool {
	if len(fieldsToRemove) > 0 {
		for _, f := range fieldsToRemove {
			if key == f || strings.HasPrefix(key, f+".") {
				return true
			}
		}
	}

	return false
}

// ExtractMongoPortAndParameters handles backward compatibility for MongoDB connection configuration.
// MONGO_PORT=5703/replicaSet=rs0&authSource=admin&directConnection=true
//
// This function extracts the actual port and parameters from such configurations.
// If MONGO_PARAMETERS is already set, it takes precedence over embedded parameters.
//
// DEPRECATED: This backward compatibility for embedded parameters in MONGO_PORT. Use the MONGO_PARAMETERS environment variable instead.
func ExtractMongoPortAndParameters(port, parameters string, logger libLog.Logger) (string, string) {
	actualPort := port
	if idx := strings.IndexAny(port, "/?"); idx != -1 {
		actualPort = port[:idx]
		embeddedParams := port[idx+1:]

		if parameters != "" {
			logger.Warnf(
				"DEPRECATED: MongoDB parameters embedded in MONGO_PORT detected but ignored "+
					"(MONGO_PARAMETERS takes precedence). Remove embedded parameters from MONGO_PORT. "+
					"Sanitized port=%s, ignored embedded=%s, using explicit=%s",
				actualPort, embeddedParams, parameters,
			)

			return actualPort, parameters
		}

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
	connectionString := fmt.Sprintf("%s://%s:%s@%s:%s/", uri, user, password, host, port)

	if parameters != "" {
		connectionString += "?" + parameters
	}

	if logger != nil {
		maskedConnStr := fmt.Sprintf("%s://<credentials>@%s:%s/", uri, host, port)
		if parameters != "" {
			maskedConnStr += "?" + parameters
		}

		logger.Debugf("MongoDB connection string built: %s", maskedConnStr)
	}

	return connectionString
}
