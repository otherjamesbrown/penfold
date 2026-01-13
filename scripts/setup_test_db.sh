#!/bin/bash

# Setup script for Penfold test database with PostgreSQL + pgvector
# This script sets up the test database infrastructure for validation

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}🔧 Setting up Penfold test database...${NC}"

# Check if PostgreSQL is installed
if ! command -v psql &> /dev/null; then
    echo -e "${RED}❌ PostgreSQL is not installed. Please install PostgreSQL 16+ first.${NC}"
    echo -e "${YELLOW}On macOS: brew install postgresql@16${NC}"
    echo -e "${YELLOW}On Ubuntu: sudo apt-get install postgresql-16 postgresql-contrib-16${NC}"
    exit 1
fi

# Check PostgreSQL version
PG_VERSION=$(psql --version | grep -oE '[0-9]+\.[0-9]+' | head -1)
PG_MAJOR=$(echo "$PG_VERSION" | cut -d. -f1)

if [ "$PG_MAJOR" -lt 14 ]; then
    echo -e "${YELLOW}⚠️  PostgreSQL version $PG_VERSION detected. Recommended: 16+${NC}"
fi

echo -e "${GREEN}✅ PostgreSQL $PG_VERSION found${NC}"

# Start PostgreSQL service if not running
if ! pg_isready -q; then
    echo -e "${YELLOW}🚀 Starting PostgreSQL service...${NC}"

    # Try different ways to start PostgreSQL
    if command -v brew &> /dev/null; then
        brew services start postgresql@16 || brew services start postgresql
    elif command -v systemctl &> /dev/null; then
        sudo systemctl start postgresql
    else
        echo -e "${YELLOW}⚠️  Please start PostgreSQL manually${NC}"
    fi

    # Wait for PostgreSQL to be ready
    echo -e "${YELLOW}⏳ Waiting for PostgreSQL to be ready...${NC}"
    for i in {1..30}; do
        if pg_isready -q; then
            break
        fi
        sleep 1
    done

    if ! pg_isready -q; then
        echo -e "${RED}❌ PostgreSQL failed to start${NC}"
        exit 1
    fi
fi

echo -e "${GREEN}✅ PostgreSQL is running${NC}"

# Set test database variables
TEST_DB="penfold_test"
TEST_USER=$(whoami)

echo -e "${YELLOW}🗄️  Setting up test database: $TEST_DB${NC}"

# Create test database (drop if exists)
psql -d postgres -c "DROP DATABASE IF EXISTS $TEST_DB;" 2>/dev/null || true
psql -d postgres -c "CREATE DATABASE $TEST_DB;" || {
    echo -e "${RED}❌ Failed to create test database${NC}"
    exit 1
}

echo -e "${GREEN}✅ Created test database: $TEST_DB${NC}"

# Install pgvector extension
echo -e "${YELLOW}📦 Installing pgvector extension...${NC}"

# Check if pgvector is available
if psql -d "$TEST_DB" -c "CREATE EXTENSION IF NOT EXISTS vector;" 2>/dev/null; then
    echo -e "${GREEN}✅ pgvector extension installed${NC}"
else
    echo -e "${RED}❌ pgvector extension not available${NC}"
    echo -e "${YELLOW}Installing pgvector...${NC}"

    if command -v brew &> /dev/null; then
        echo -e "${YELLOW}Installing via Homebrew...${NC}"
        brew install pgvector
    else
        echo -e "${YELLOW}Please install pgvector manually:${NC}"
        echo -e "${YELLOW}  - Clone: git clone --branch v0.6.0 https://github.com/pgvector/pgvector.git${NC}"
        echo -e "${YELLOW}  - Build: cd pgvector && make && sudo make install${NC}"
    fi

    # Try again
    if ! psql -d "$TEST_DB" -c "CREATE EXTENSION IF NOT EXISTS vector;" 2>/dev/null; then
        echo -e "${RED}❌ Failed to install pgvector extension${NC}"
        echo -e "${YELLOW}⚠️  Vector operations will not work without pgvector${NC}"
    else
        echo -e "${GREEN}✅ pgvector extension installed${NC}"
    fi
fi

# Install other required extensions
echo -e "${YELLOW}📦 Installing additional extensions...${NC}"

psql -d "$TEST_DB" -c "CREATE EXTENSION IF NOT EXISTS ltree;" || echo -e "${YELLOW}⚠️  ltree extension not available${NC}"
psql -d "$TEST_DB" -c "CREATE EXTENSION IF NOT EXISTS pg_trgm;" || echo -e "${YELLOW}⚠️  pg_trgm extension not available${NC}"

echo -e "${GREEN}✅ Database extensions configured${NC}"

# Set up environment variables for testing
echo -e "${YELLOW}🔧 Setting up environment variables...${NC}"

cat > .env.test << EOF
# Test database configuration
DB_HOST=localhost
DB_PORT=5432
DB_NAME=$TEST_DB
DB_USER=$TEST_USER
DB_PASSWORD=

# Test Redis configuration (use different DB for isolation)
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_DB=15

# Test environment flag
PENFOLD_ENV=testing
EOF

echo -e "${GREEN}✅ Created .env.test file${NC}"

# Test database connection
echo -e "${YELLOW}🧪 Testing database connection...${NC}"

if psql -d "$TEST_DB" -c "SELECT version();" > /dev/null 2>&1; then
    echo -e "${GREEN}✅ Database connection successful${NC}"
else
    echo -e "${RED}❌ Database connection failed${NC}"
    exit 1
fi

# Test pgvector functionality
echo -e "${YELLOW}🧪 Testing pgvector functionality...${NC}"

if psql -d "$TEST_DB" -c "SELECT '[1,2,3]'::vector;" > /dev/null 2>&1; then
    echo -e "${GREEN}✅ pgvector working correctly${NC}"
else
    echo -e "${YELLOW}⚠️  pgvector test failed - vector operations may not work${NC}"
fi

# Check Redis connection (optional)
echo -e "${YELLOW}🧪 Testing Redis connection...${NC}"

if command -v redis-cli &> /dev/null; then
    if redis-cli ping > /dev/null 2>&1; then
        echo -e "${GREEN}✅ Redis connection successful${NC}"
    else
        echo -e "${YELLOW}⚠️  Redis not available - event processing tests may fail${NC}"
        echo -e "${YELLOW}To install Redis: brew install redis (macOS) or apt-get install redis (Ubuntu)${NC}"
    fi
else
    echo -e "${YELLOW}⚠️  Redis CLI not found - install Redis for full testing${NC}"
fi

echo -e "\n${GREEN}🎉 Test database setup complete!${NC}"
echo -e "\n${YELLOW}📋 Summary:${NC}"
echo -e "  📊 Database: $TEST_DB"
echo -e "  👤 User: $TEST_USER"
echo -e "  📁 Config: .env.test"
echo -e "\n${YELLOW}🚀 Next steps:${NC}"
echo -e "  1. Run unit tests: python3.12 -m pytest tests/unit/ -v"
echo -e "  2. Run integration tests: python3.12 -m pytest tests/integration/ -v"
echo -e "  3. Run performance tests: python3.12 -m pytest tests/performance/ -v"
echo -e "\n${GREEN}🔗 Test commands:${NC}"
echo -e "  export $(cat .env.test | xargs) && python3.12 -m pytest tests/ -v"
echo ""