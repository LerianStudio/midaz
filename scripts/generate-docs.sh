#!/bin/bash
#!/usr/bin/env bash
set -euo pipefail

# Root directory of the repo
default_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Components to process
components=("onboarding" "transaction")

# Temporary log dir
log_dir="$default_root/tmp/generate-docs-logs"
mkdir -p "$log_dir"

echo "Generating Swagger documentation for all services..."

for comp in "${components[@]}"; do
  echo " - $comp"
  comp_dir="$default_root/components/$comp"
  out_log="$log_dir/$comp.out"
  err_log="$log_dir/$comp.err"

  # Run component's docs generation quietly
  pushd "$comp_dir" >/dev/null
  make generate-docs -s > "$out_log" 2> "$err_log" || true
  popd >/dev/null

done

# Summary of Swagger documentation generation
echo
for comp in "${components[@]}"; do
  out_log="$log_dir/$comp.out"
  err_log="$log_dir/$comp.err"
  w=$(grep -E "(warning:|WARN)" "$out_log" "$err_log" | wc -l || true)
  e=$(grep -E "(error:|ERROR)" "$out_log" "$err_log" | grep -v "warning:" | wc -l || true)
  if [[ $e -gt 0 ]]; then
    echo " - $comp: failed (${e} errors, ${w} warnings)"
    echo "   Errors for $comp:"
    grep -E "(error:|ERROR)" "$out_log" "$err_log" | grep -v "warning:" || true
  else
    echo " - $comp: success (${w} warnings)"
  fi
done

echo -e "\nSyncing Postman collection with the generated OpenAPI documentation..."

sync_out="$log_dir/sync.out"
sync_err="$log_dir/sync.err"
if ! "$(dirname "$0")/sync-postman.sh" > "$sync_out" 2> "$sync_err"; then
  echo "Error syncing Postman collection. See $sync_err"
  exit 1
fi
echo "Postman collection synced successfully."
echo "Cleaning up temporary logs..."
rm -rf "$default_root/tmp"

exit 0
