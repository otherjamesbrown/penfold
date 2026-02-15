#!/bin/bash

# Setup script for Penfold test database with PostgreSQL + pgvector
# This script sets up the test database infrastructure on dev02.brown.chat

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}🔧 Setting up Penfold test database on dev02.brown.chat...${NC}"

# Connection settings
DB_HOST="${PENFOLD_DB_HOST:-dev02.brown.chat}"
DB_PORT="${PENFOLD_DB_PORT:-5432}"
DB_USER="${PENFOLD_DB_USER:-penfold}"

# SSL cert paths
HOME_DIR="$HOME"
SSL_CERT="${HOME_DIR}/.postgresql/postgresql.crt"
SSL_KEY="${HOME_DIR}/.postgresql/postgresql.key"
SSL_ROOT_CERT="${HOME_DIR}/.postgresql/root.crt"

# Check if SSL certs exist
if [ ! -f "$SSL_CERT" ] || [ ! -f "$SSL_KEY" ] || [ ! -f "$SSL_ROOT_CERT" ]; then
    echo -e "${RED}❌ SSL certificates not found in ~/.postgresql/${NC}"
    echo -e "${YELLOW}Expected files:${NC}"
    echo -e "  - ${SSL_CERT}"
    echo -e "  - ${SSL_KEY}"
    echo -e "  - ${SSL_ROOT_CERT}"
    exit 1
fi

echo -e "${GREEN}✅ SSL certificates found${NC}"

# Check if psql is installed
if ! command -v psql &> /dev/null; then
    echo -e "${RED}❌ psql is not installed. Please install PostgreSQL client first.${NC}"
    echo -e "${YELLOW}On macOS: brew install postgresql@16${NC}"
    echo -e "${YELLOW}On Ubuntu: sudo apt-get install postgresql-client-16${NC}"
    exit 1
fi

# Build connection string for remote database
CONN_STR="postgresql://${DB_USER}@${DB_HOST}:${DB_PORT}/postgres?sslmode=verify-full&sslcert=${SSL_CERT}&sslkey=${SSL_KEY}&sslrootcert=${SSL_ROOT_CERT}"

# Test connection
echo -e "${YELLOW}🧪 Testing connection to ${DB_HOST}...${NC}"
if ! psql "${CONN_STR}" -c "SELECT version();" > /dev/null 2>&1; then
    echo -e "${RED}❌ Failed to connect to ${DB_HOST}${NC}"
    exit 1
fi

echo -e "${GREEN}✅ Connected to ${DB_HOST}${NC}"

# Set test database name
TEST_DB="penfold_test"

echo -e "${YELLOW}🗄️  Setting up test database: $TEST_DB${NC}"

# Create test database (drop if exists)
psql "${CONN_STR}" -c "DROP DATABASE IF EXISTS ${TEST_DB};" 2>/dev/null || true
psql "${CONN_STR}" -c "CREATE DATABASE ${TEST_DB};" || {
    echo -e "${RED}❌ Failed to create test database${NC}"
    exit 1
}

echo -e "${GREEN}✅ Created test database: $TEST_DB${NC}"

# Build connection string for test database
TEST_CONN_STR="postgresql://${DB_USER}@${DB_HOST}:${DB_PORT}/${TEST_DB}?sslmode=verify-full&sslcert=${SSL_CERT}&sslkey=${SSL_KEY}&sslrootcert=${SSL_ROOT_CERT}"

# Install pgvector extension
echo -e "${YELLOW}📦 Installing pgvector extension...${NC}"

if psql "${TEST_CONN_STR}" -c "CREATE EXTENSION IF NOT EXISTS vector;" 2>/dev/null; then
    echo -e "${GREEN}✅ pgvector extension installed${NC}"
else
    echo -e "${RED}❌ pgvector extension not available on ${DB_HOST}${NC}"
    echo -e "${YELLOW}⚠️  Contact database administrator to install pgvector${NC}"
fi

# Install other required extensions
echo -e "${YELLOW}📦 Installing additional extensions...${NC}"

psql "${TEST_CONN_STR}" -c "CREATE EXTENSION IF NOT EXISTS ltree;" || echo -e "${YELLOW}⚠️  ltree extension not available${NC}"
psql "${TEST_CONN_STR}" -c "CREATE EXTENSION IF NOT EXISTS pg_trgm;" || echo -e "${YELLOW}⚠️  pg_trgm extension not available${NC}"

echo -e "${GREEN}✅ Database extensions configured${NC}"

# Set up environment variables for testing
echo -e "${YELLOW}🔧 Setting up environment variables...${NC}"

cat > .env.test << EOF
# Test database configuration (remote dev02)
PENFOLD_DB_HOST=${DB_HOST}
PENFOLD_DB_PORT=${DB_PORT}
PENFOLD_DB_USER=${DB_USER}
PENFOLD_DB_NAME=${TEST_DB}

# SSL configuration
PENFOLD_DB_SSLMODE=verify-full
PENFOLD_DB_SSLCERT=${SSL_CERT}
PENFOLD_DB_SSLKEY=${SSL_KEY}
PENFOLD_DB_SSLROOTCERT=${SSL_ROOT_CERT}

# Test environment flag
PENFOLD_ENV=testing
EOF

echo -e "${GREEN}✅ Created .env.test file${NC}"

# Test database connection
echo -e "${YELLOW}🧪 Testing database connection...${NC}"

if psql "${TEST_CONN_STR}" -c "SELECT version();" > /dev/null 2>&1; then
    echo -e "${GREEN}✅ Database connection successful${NC}"
else
    echo -e "${RED}❌ Database connection failed${NC}"
    exit 1
fi

# Test pgvector functionality
echo -e "${YELLOW}🧪 Testing pgvector functionality...${NC}"

if psql "${TEST_CONN_STR}" -c "SELECT '[1,2,3]'::vector;" > /dev/null 2>&1; then
    echo -e "${GREEN}✅ pgvector working correctly${NC}"
else
    echo -e "${YELLOW}⚠️  pgvector test failed - vector operations may not work${NC}"
fi

echo -e "\n${GREEN}🎉 Test database setup complete on ${DB_HOST}!${NC}"
echo -e "\n${YELLOW}📋 Summary:${NC}"
echo -e "  🖥️  Host: ${DB_HOST}:${DB_PORT}"
echo -e "  📊 Database: ${TEST_DB}"
echo -e "  👤 User: ${DB_USER}"
echo -e "  🔐 SSL: Enabled (client cert auth)"
echo -e "  📁 Config: .env.test"
echo -e "\n${YELLOW}🚀 Next steps:${NC}"
echo -e "  1. Run E2E tests: go test -tags=e2e ./tests/e2e/... -v"
echo -e "  2. Run integration tests: go test -tags=integration ./tests/integration/... -v"
echo -e "  3. Run quality tests: go test -tags=quality ./tests/quality/... -v"
echo ""