#!/bin/sh
set -e

echo "waiting for mongodb at ${MONGO_HOST}:${MONGO_PORT}... ⏳ "

# Wait for MongoDB to be ready
until mongosh --host "${MONGO_HOST}" --port "${MONGO_PORT}" --eval "db.adminCommand('ping')" > /dev/null 2>&1; do
  echo "mongodb is not ready yet..."
  sleep 2
done

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

# Create the admin user
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

echo "mongodb replica set initialization complete! ✅ "
