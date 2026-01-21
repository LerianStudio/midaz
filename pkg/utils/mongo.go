package utils

import (
	libLog "github.com/LerianStudio/lib-commons/v2/commons/log"
	pkgMongo "github.com/LerianStudio/midaz/v3/pkg/mongo"
)

// ExtractMongoPortAndParameters handles backward compatibility for MongoDB connection configuration.
//
// Deprecated: This function has been moved to pkg/mongo. Use pkgMongo.ExtractMongoPortAndParameters instead.
// This forwarding wrapper will be removed in Midaz 5.0.0.
func ExtractMongoPortAndParameters(port, parameters string, logger libLog.Logger) (string, string) {
	return pkgMongo.ExtractMongoPortAndParameters(port, parameters, logger)
}

// BuildMongoConnectionString constructs a properly formatted MongoDB connection string.
//
// Deprecated: This function has been moved to pkg/mongo. Use pkgMongo.BuildMongoConnectionString instead.
// This forwarding wrapper will be removed in Midaz 5.0.0.
func BuildMongoConnectionString(uri, user, password, host, port, parameters string, logger libLog.Logger) string {
	return pkgMongo.BuildMongoConnectionString(uri, user, password, host, port, parameters, logger)
}
