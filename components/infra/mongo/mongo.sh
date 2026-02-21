#!/bin/sh

# Copyright (c) 2026 Lerian Studio. All rights reserved.
# Use of this source code is governed by the Elastic License 2.0
# that can be found in the LICENSE file.

set -e

echo "waiting for mongodb at ${MONGO_HOST}:${MONGO_PORT}... ⏳ "

# Wait for MongoDB to be ready
until mongosh --host "${MONGO_HOST}" --port "${MONGO_PORT}" --eval "db.adminCommand('ping')" > /dev/null 2>&1; do
  echo "mongodb is not ready yet..."
  sleep 2
done

echo "initializing replica set... 🚀 "

# Initialize the replica set (idempotent: skip if already initialized)
mongosh --host "${MONGO_HOST}" --port "${MONGO_PORT}" <<EOF
    var status = rs.status();
    if (status.ok === 0) {
      rs.initiate({
        _id: "rs0",
        members: [{ _id: 0, host: "${MONGO_HOST}:${MONGO_PORT}" }]
      });
      print("Replica set initialized.");
    } else {
      print("Replica set already initialized, skipping.");
    }
EOF

# Wait for the replica set to initialize
sleep 5

# Create or update the admin user (idempotent)
echo "creating admin user... 🔑"
mongosh --host "${MONGO_HOST}" --port "${MONGO_PORT}" <<EOF
    use admin;
    if (db.getUser("${MONGO_USER}") === null) {
      db.createUser({
        user: "${MONGO_USER}",
        pwd: "${MONGO_PASSWORD}",
        roles: [
          { role: "dbAdminAnyDatabase", db: "admin" },
          { role: "readWriteAnyDatabase", db: "admin" }
        ]
      });
      print("User created successfully.");
    } else {
      db.updateUser("${MONGO_USER}", {
        roles: [
          { role: "dbAdminAnyDatabase", db: "admin" },
          { role: "readWriteAnyDatabase", db: "admin" }
        ]
      });
      print("User already exists, roles updated.");
    }
EOF

echo "mongodb replica set initialization complete! ✅ "
