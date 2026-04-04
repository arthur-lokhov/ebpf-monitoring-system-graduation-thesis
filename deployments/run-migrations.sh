#!/bin/bash
# Run database migrations

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MIGRATION_FILE="$SCRIPT_DIR/../internal/storage/postgres/migrations/001_init_schema.up.sql"

echo "Running database migrations..."
docker exec -i epbf-postgres psql -U epbf -d epbf -f - < "$MIGRATION_FILE"

echo "✅ Migrations completed successfully!"
