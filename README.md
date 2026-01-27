# API Service

Go REST API for the Multi-Tenant SaaS application.

## Development

### Prerequisites

- Go 1.21+
- PostgreSQL 15+
- Redis 7+

### Setup

```bash
# Install dependencies
go mod download

# Run migrations
goose -dir migrations postgres "postgres://saas_user:saas_password@localhost:5432/saas_db?sslmode=disable" up

# Run server
go run cmd/api/main.go
```

### Testing

```bash
# Run all tests
go test ./...

# Run with coverage
go test -v -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run specific test
go test -v ./internal/services -run TestProjectService_TenantIsolation
```

### Creating Migrations

```bash
# Create a new migration
goose -dir migrations create add_new_table sql

# Edit the generated file, then run:
goose -dir migrations postgres "postgres://saas_user:saas_password@localhost:5432/saas_db?sslmode=disable" up
```

## API Endpoints

See the main README.md for detailed API examples and curl commands.

## Architecture

- **Router**: Chi router with middleware for auth, tenant scoping, and RBAC
- **Services**: Business logic layer
- **Handlers**: HTTP request/response handling
- **Database**: PostgreSQL with row-level tenancy
- **Sessions**: Redis-backed HttpOnly cookie sessions
