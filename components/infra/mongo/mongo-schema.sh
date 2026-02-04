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

# Check if admin user already exists
echo "checking admin user... 🔑"
USER_EXISTS=$(mongosh --host "${MONGO_HOST}" --port "${MONGO_PORT}" --quiet --eval "use admin; db.getUsers().users.filter(u => u.user === '${MONGO_USER}').length" 2>/dev/null || echo "0")

if [ "$USER_EXISTS" = "0" ]; then
  echo "creating admin user... 🔑"
  mongosh --host "${MONGO_HOST}" --port "${MONGO_PORT}" <<EOF
      use admin;
      db.createUser({
        user: "${MONGO_USER}",
        pwd: "${MONGO_PASSWORD}",
        roles: [
          { role: "root", db: "admin" },
          { role: "userAdminAnyDatabase", db: "admin" },
          { role: "dbAdminAnyDatabase", db: "admin" },
          { role: "readWriteAnyDatabase", db: "admin" }
        ]
      });
EOF
  echo "admin user created! ✅"
else
  echo "admin user already exists, skipping... ✅"
fi

echo "mongodb per-schema initialization complete! ✅ "
