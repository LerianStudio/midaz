#!/bin/sh
set -e

echo "waiting for mongodb at ${MONGO_HOST}:${MONGO_PORT}... ⏳ "

until mongosh --host "${MONGO_HOST}:${MONGO_PORT}" --eval "db.adminCommand('ping')" > /dev/null 2>&1; do
  echo "mongodb is not ready yet..."
  sleep 2
done

echo "initializing replica set... 🚀 "

mongosh --host "${MONGO_HOST}:${MONGO_PORT}" -u "${MONGO_USER}" -p "${MONGO_PASSWORD}" --authenticationDatabase admin <<EOF
    rs.initiate({
      _id: "rs0",
      members: [{ _id: 0, host: "${MONGO_HOST}:${MONGO_PORT}" }]
    });
EOF

echo "mongodb replica set initialization complete! ✅ "
