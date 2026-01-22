package mongo

import (
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
		embeddedParams := strings.TrimLeft(port[idx+1:], "/?")

		if parameters != "" {
			if logger != nil {
				logger.Warnf(
					"DEPRECATED: MongoDB parameters embedded in MONGO_PORT detected but ignored "+
						"(MONGO_PARAMETERS takes precedence). Remove embedded parameters from MONGO_PORT. "+
						"Sanitized port=%s",
					actualPort,
				)
			}

			return actualPort, parameters
		}

		if logger != nil {
			logger.Warnf(
				"DEPRECATED: MongoDB parameters embedded in MONGO_PORT detected. "+
					"Update environment variables to use the MONGO_PARAMETERS environment variable. "+
					"Sanitized port=%s",
				actualPort,
			)
		}

		return actualPort, embeddedParams
	}

	return actualPort, parameters
}
