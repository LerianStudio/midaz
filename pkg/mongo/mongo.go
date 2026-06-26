// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package mongo

import (
	"context"
	"fmt"
	"strings"

	libLog "github.com/LerianStudio/lib-observability/log"
	"github.com/iancoleman/strcase"
	"go.mongodb.org/mongo-driver/v2/bson"
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
//
// mongo-driver v2 decodes nested documents into bson.D (ordered), whereas v1
// produced bson.M, so both shapes must be recursed into — otherwise a nested
// document is kept whole and a dotted $unset on one of its fields collides with
// the $set on the parent ("would create a conflict at <path>").
func flattenBSONM(m bson.M, prefix string, flat bson.M) {
	for k, v := range m {
		var key string
		if prefix == "" {
			key = k
		} else {
			key = prefix + "." + k
		}

		switch sub := v.(type) {
		case bson.M:
			flattenBSONM(sub, key, flat)
		case bson.D:
			flattenBSONM(bsonDToM(sub), key, flat)
		default:
			flat[key] = v
		}
	}
}

// bsonDToM converts an ordered bson.D into a bson.M. mongo-driver v2 dropped
// bson.D.Map(), so the conversion is done explicitly.
func bsonDToM(d bson.D) bson.M {
	m := make(bson.M, len(d))
	for _, e := range d {
		m[e.Key] = e.Value
	}

	return m
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
func ExtractMongoPortAndParameters(port, parameters string, logger libLog.Logger) (string, string) {
	actualPort := port
	if idx := strings.IndexAny(port, "/?"); idx != -1 {
		actualPort = port[:idx]
		embeddedParams := strings.TrimLeft(port[idx+1:], "/?")

		if parameters != "" {
			if logger != nil {
				logger.Log(
					context.Background(),
					libLog.LevelWarn,
					fmt.Sprintf(
						"MongoDB parameters embedded in MONGO_PORT detected but ignored "+
							"(MONGO_PARAMETERS takes precedence). Remove embedded parameters from MONGO_PORT. "+
							"Sanitized port=%s",
						actualPort,
					),
				)
			}

			return actualPort, parameters
		}

		if logger != nil {
			logger.Log(
				context.Background(),
				libLog.LevelWarn,
				fmt.Sprintf(
					"MongoDB parameters embedded in MONGO_PORT detected. "+
						"Update environment variables to use the MONGO_PARAMETERS environment variable. "+
						"Sanitized port=%s",
					actualPort,
				),
			)
		}

		return actualPort, embeddedParams
	}

	return actualPort, parameters
}
