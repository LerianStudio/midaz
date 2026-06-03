#!/bin/sh
# Creates the default storage bucket in SeaweedFS if it does not exist.
# Called by `make up` after infrastructure services are healthy.

BUCKET="${OBJECT_STORAGE_BUCKET:-reporter-storage}"
SEAWEEDFS_URL="http://localhost:8333"
MAX_RETRIES=5

for i in $(seq 1 "$MAX_RETRIES"); do
  STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X PUT "${SEAWEEDFS_URL}/${BUCKET}" 2>/dev/null)

  if [ "$STATUS" = "200" ]; then
    echo "Bucket '${BUCKET}' created in SeaweedFS"
    exit 0
  elif [ "$STATUS" = "409" ]; then
    echo "Bucket '${BUCKET}' already exists in SeaweedFS"
    exit 0
  fi

  echo "Waiting for SeaweedFS S3 API... (attempt ${i}/${MAX_RETRIES}, status: ${STATUS})"
  sleep 2
done

echo "[error] Failed to create bucket '${BUCKET}' after ${MAX_RETRIES} attempts"
exit 1
