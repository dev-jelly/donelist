# Developer Onboarding Guide

Welcome to the DoneList API development team! This guide will help you get set up and productive quickly.

## Table of Contents

1. [Quick Start](#quick-start)
2. [Development Environment](#development-environment)
3. [Project Structure](#project-structure)
4. [Development Workflow](#development-workflow)
5. [Testing Strategy](#testing-strategy)
6. [Code Standards](#code-standards)
7. [Common Tasks](#common-tasks)
8. [Troubleshooting](#troubleshooting)
9. [Resources](#resources)

## Quick Start

### Prerequisites Checklist

- [ ] Go 1.24+ installed
- [ ] Docker and Docker Compose installed
- [ ] PostgreSQL 15+ client tools
- [ ] Git configured with SSH keys
- [ ] Code editor (VS Code recommended)
- [ ] Make installed

### 5-Minute Setup

```bash
# 1. Clone repository
git clone git@github.com:dev-jelly/donelist.git
cd donelist/server

# 2. Copy environment file
cp .env.example .env

# 3. Start dependencies
docker-compose up -d postgres redis

# 4. Install dependencies
go mod download

# 5. Run migrations
make migrate

# 6. Start API server
make run

# 7. Verify it works
curl http://localhost:8080/health
```

**Expected Output:**
```json
{
  "status": "healthy",
  "timestamp": "2025-11-24T10:00:00Z",
  "version": "1.0.0"
}
```

For complete documentation, see docs/DEVELOPER_ONBOARDING.md in the repository.
