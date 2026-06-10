// Copyright (c) 2026 Lerian Studio. All rights reserved.
// Use of this source code is governed by the Elastic License 2.0
// that can be found in the LICENSE file.

package bootstrap

import (
	"context"
	"fmt"
	"net/url"

	libMongo "github.com/LerianStudio/lib-commons/v5/commons/mongo"
	clog "github.com/LerianStudio/lib-observability/log"

	pkg "github.com/LerianStudio/midaz/v4/pkg/reporter"
	pkgConstant "github.com/LerianStudio/midaz/v4/pkg/reporter/constant"
	mongoDB "github.com/LerianStudio/midaz/v4/pkg/reporter/mongodb"
)

// buildMongoConnection creates a MongoConnection with the connection string
// built from configuration, applying default pool size if needed.
func buildMongoConnection(cfg *Config, logger clog.Logger) *mongoDB.MongoConnection {
	escapedPass := url.QueryEscape(cfg.MongoDBPassword)
	mongoSource := fmt.Sprintf("%s://%s:%s@%s:%s",
		cfg.MongoURI, cfg.MongoDBUser, escapedPass, cfg.MongoDBHost, cfg.MongoDBPort)

	if cfg.MongoDBParameters != "" {
		mongoSource += "/?" + cfg.MongoDBParameters
	}

	maxPoolSize := cfg.MaxPoolSize
	if maxPoolSize <= 0 {
		maxPoolSize = int(pkgConstant.MongoDBMaxPoolSize)
	}

	logger.Log(context.Background(), clog.LevelInfo, "MongoDB connecting", clog.String("dsn", pkg.RedactConnectionString(mongoSource)))

	var mongoTLS *libMongo.TLSConfig
	if cfg.MongoTLSCACert != "" {
		mongoTLS = &libMongo.TLSConfig{CACertBase64: cfg.MongoTLSCACert}
	}

	return &mongoDB.MongoConnection{
		ConnectionStringSource: mongoSource,
		Database:               cfg.MongoDBName,
		Logger:                 logger,
		MaxPoolSize:            uint64(maxPoolSize), // #nosec G115 -- maxPoolSize is guaranteed positive by the guard above
		TLS:                    mongoTLS,
	}
}
