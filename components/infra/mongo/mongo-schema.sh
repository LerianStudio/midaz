#!/bin/sh
set -e

echo "waiting for mongodb at ${MONGO_HOST}:${MONGO_PORT}... ⏳ "

# Wait for MongoDB to be ready
until mongosh --host "${MONGO_HOST}" --port "${MONGO_PORT}" --eval "db.adminCommand('ping')" > /dev/null 2>&1; do
  echo "mongodb is not ready yet..."
  sleep 2
done

echo "checking replica set status... 🔍"

# Check if replica set is already initialized
RS_STATUS=$(mongosh --host "${MONGO_HOST}" --port "${MONGO_PORT}" --quiet --eval "try { rs.status().ok } catch(e) { 0 }" 2>/dev/null || echo "0")

if [ "$RS_STATUS" = "1" ]; then
  echo "replica set already initialized, checking admin user..."
else
  echo "initializing replica set... 🚀 "

  # Initialize the replica set
  mongosh --host "${MONGO_HOST}" --port "${MONGO_PORT}" <<EOF
      rs.initiate({
        _id: "rs0",
        members: [{ _id: 0, host: "${MONGO_HOST}:${MONGO_PORT}" }]
      });
EOF

  # Wait for the replica set to initialize
  sleep 5
fi

# Create admin user (idempotent - skips if already exists)
echo "creating admin user... 🔑"
mongosh --host "${MONGO_HOST}" --port "${MONGO_PORT}" --eval '
try {
  db.getSiblingDB("admin").createUser({
    user: "'"${MONGO_USER}"'",
    pwd: "'"${MONGO_PASSWORD}"'",
    roles: [
      { role: "root", db: "admin" },
      { role: "userAdminAnyDatabase", db: "admin" },
      { role: "dbAdminAnyDatabase", db: "admin" },
      { role: "readWriteAnyDatabase", db: "admin" }
    ]
  });
  print("admin user created! ✅");
} catch(e) {
  if (e.codeName === "DuplicateKey" || e.message.includes("already exists")) {
    print("admin user already exists, skipping... ✅");
  } else {
    print("ERROR: " + e.message);
    throw e;
  }
}
'

echo "mongodb per-schema initialization complete! ✅ "
