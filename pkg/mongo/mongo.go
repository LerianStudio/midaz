package mongo

import (
	"strings"

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
