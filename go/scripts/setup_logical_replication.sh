#!/bin/bash
#
# setup_logical_replication.sh
#
# Sets up PostgreSQL logical replication for the syncdata module.
# This script configures the production database to output change files
# that can be consumed by the syncdata utility.
#
# Prerequisites:
#   - PostgreSQL 10+ with wal2json extension installed
#   - Superuser access to PostgreSQL
#   - PG_* environment variables set
#
# Usage:
#   ./setup_logical_replication.sh [--create-slot] [--create-user]
#

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Default values
CREATE_SLOT=false
CREATE_USER=false
SLOT_NAME="syncdata_slot"
SYNC_USER="syncdata_user"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --create-slot)
            CREATE_SLOT=true
            shift
            ;;
        --create-user)
            CREATE_USER=true
            shift
            ;;
        --slot-name)
            SLOT_NAME="$2"
            shift 2
            ;;
        --user-name)
            SYNC_USER="$2"
            shift 2
            ;;
        -h|--help)
            echo "Usage: $0 [options]"
            echo ""
            echo "Options:"
            echo "  --create-slot    Create the logical replication slot"
            echo "  --create-user    Create the replication user"
            echo "  --slot-name      Name of the replication slot (default: syncdata_slot)"
            echo "  --user-name      Name of the replication user (default: syncdata_user)"
            echo "  -h, --help       Show this help"
            exit 0
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            exit 1
            ;;
    esac
done

# Check required environment variables
check_env() {
    local var_name=$1
    local var_value="${!var_name}"
    if [[ -z "$var_value" ]]; then
        echo -e "${RED}Error: $var_name is not set${NC}"
        exit 1
    fi
}

echo -e "${GREEN}=== PostgreSQL Logical Replication Setup ===${NC}"
echo ""

# Check PG environment
check_env "PG_USER_NAME"
check_env "PG_PASSWORD"
check_env "PG_DB_NAME"

PG_HOST="${PG_HOST:-127.0.0.1}"
PG_PORT="${PG_PORT:-5432}"

export PGPASSWORD="$PG_PASSWORD"

echo "Database: $PG_DB_NAME @ $PG_HOST:$PG_PORT"
echo ""

# Function to run psql
run_psql() {
    psql -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER_NAME" -d "$PG_DB_NAME" -t -A -c "$1"
}

run_psql_admin() {
    psql -h "$PG_HOST" -p "$PG_PORT" -U "$PG_USER_NAME" -d postgres -t -A -c "$1"
}

# Step 1: Check current wal_level
echo -e "${YELLOW}Checking current PostgreSQL configuration...${NC}"

WAL_LEVEL=$(run_psql "SHOW wal_level;")
ARCHIVE_MODE=$(run_psql "SHOW archive_mode;")

echo "  wal_level: $WAL_LEVEL"
echo "  archive_mode: $ARCHIVE_MODE"
echo ""

if [[ "$WAL_LEVEL" != "logical" ]]; then
    echo -e "${YELLOW}Warning: wal_level is '$WAL_LEVEL', needs to be 'logical'${NC}"
    echo ""
    echo "To enable logical replication, run these commands as superuser:"
    echo ""
    echo "  ALTER SYSTEM SET wal_level = 'logical';"
    echo ""
    echo "Then restart PostgreSQL:"
    echo ""
    echo "  # macOS Homebrew"
    echo "  brew services restart postgresql@18"
    echo ""
    echo "  # Linux systemd"
    echo "  sudo systemctl restart postgresql"
    echo ""

    read -p "Do you want to set wal_level = 'logical' now? [y/N] " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        run_psql "ALTER SYSTEM SET wal_level = 'logical';"
        echo -e "${GREEN}wal_level set to 'logical'. Please restart PostgreSQL.${NC}"
    fi
else
    echo -e "${GREEN}wal_level is already set to 'logical'${NC}"
fi

# Step 2: Check for wal2json extension
echo ""
echo -e "${YELLOW}Checking for wal2json extension...${NC}"

HAS_WAL2JSON=$(run_psql "SELECT COUNT(*) FROM pg_available_extensions WHERE name = 'wal2json';" 2>/dev/null || echo "0")

if [[ "$HAS_WAL2JSON" == "0" ]]; then
    echo -e "${YELLOW}Warning: wal2json extension not found${NC}"
    echo ""
    echo "To install wal2json:"
    echo ""
    echo "  # macOS Homebrew"
    echo "  brew install wal2json"
    echo ""
    echo "  # Debian/Ubuntu"
    echo "  apt-get install postgresql-16-wal2json"
    echo ""
    echo "  # From source"
    echo "  git clone https://github.com/eulerto/wal2json.git"
    echo "  cd wal2json && make && make install"
    echo ""
else
    echo -e "${GREEN}wal2json extension is available${NC}"
fi

# Step 3: Create replication user
if [[ "$CREATE_USER" == true ]]; then
    echo ""
    echo -e "${YELLOW}Creating replication user '$SYNC_USER'...${NC}"

    USER_EXISTS=$(run_psql_admin "SELECT COUNT(*) FROM pg_roles WHERE rolname = '$SYNC_USER';")

    if [[ "$USER_EXISTS" == "0" ]]; then
        read -s -p "Enter password for $SYNC_USER: " SYNC_PASSWORD
        echo

        run_psql_admin "CREATE USER $SYNC_USER WITH REPLICATION LOGIN PASSWORD '$SYNC_PASSWORD';"
        run_psql "GRANT SELECT ON ALL TABLES IN SCHEMA public TO $SYNC_USER;"

        echo -e "${GREEN}User '$SYNC_USER' created with REPLICATION privilege${NC}"
    else
        echo -e "${GREEN}User '$SYNC_USER' already exists${NC}"

        # Ensure REPLICATION privilege
        run_psql_admin "ALTER USER $SYNC_USER WITH REPLICATION;"
        echo "  REPLICATION privilege granted"
    fi
fi

# Step 4: Create replication slot
if [[ "$CREATE_SLOT" == true ]]; then
    echo ""
    echo -e "${YELLOW}Creating logical replication slot '$SLOT_NAME'...${NC}"

    SLOT_EXISTS=$(run_psql "SELECT COUNT(*) FROM pg_replication_slots WHERE slot_name = '$SLOT_NAME';")

    if [[ "$SLOT_EXISTS" == "0" ]]; then
        run_psql "SELECT pg_create_logical_replication_slot('$SLOT_NAME', 'wal2json');"
        echo -e "${GREEN}Replication slot '$SLOT_NAME' created${NC}"
    else
        echo -e "${GREEN}Replication slot '$SLOT_NAME' already exists${NC}"
    fi
fi

# Step 5: Show current slots
echo ""
echo -e "${YELLOW}Current replication slots:${NC}"
run_psql "SELECT slot_name, plugin, slot_type, active FROM pg_replication_slots;" | while read line; do
    echo "  $line"
done

echo ""
echo -e "${GREEN}=== Setup Complete ===${NC}"
echo ""
echo "Next steps:"
echo "  1. If wal_level was changed, restart PostgreSQL"
echo "  2. Run archive_changes.sh to start archiving changes"
echo "  3. Configure syncdata with the archive location"
echo ""
