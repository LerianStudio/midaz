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

echo "initializing replica set... 🚀"

# Initialize the replica set (idempotent: handles NotYetInitialized)
mongosh --quiet --host "${MONGO_HOST}" --port "${MONGO_PORT}" <<EOF
try {
  rs.status();
  print("Replica set already initialized, skipping.");
} catch (e) {
  if (e.codeName === "NotYetInitialized") {
    rs.initiate({
      _id: "rs0",
      members: [{ _id: 0, host: "${MONGO_HOST}:${MONGO_PORT}" }]
    });
    print("Replica set initialized.");
  } else {
    throw e;
  }
}
EOF

echo "waiting for replica set PRIMARY..."
until mongosh --quiet --host "${MONGO_HOST}" --port "${MONGO_PORT}" --eval '
const hello = db.hello();
if (hello.isWritablePrimary) {
  quit(0);
}
quit(1);
' > /dev/null 2>&1; do
  echo "replica set is not primary yet..."
  sleep 2
done

# Create or update the admin user (idempotent)
echo "creating admin user... 🔑"
mongosh --quiet --host "${MONGO_HOST}" --port "${MONGO_PORT}" <<EOF
const admin = db.getSiblingDB("admin");
const roles = [
  { role: "dbAdminAnyDatabase", db: "admin" },
  { role: "readWriteAnyDatabase", db: "admin" }
];

if (admin.getUser("${MONGO_USER}") === null) {
  admin.createUser({
    user: "${MONGO_USER}",
    pwd: "${MONGO_PASSWORD}",
    roles: roles
  });
  print("User created successfully.");
} else {
  admin.updateUser("${MONGO_USER}", {
    pwd: "${MONGO_PASSWORD}",
    roles: roles
  });
  print("User already exists, credentials/roles updated.");
}
EOF

echo "mongodb replica set initialization complete! ✅ "
