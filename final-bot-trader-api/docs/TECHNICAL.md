# Final Bot Trader API - Technical Documentation

## Overview

Final Bot Trader API is a backend service for managing an automated trading bot system. The API provides endpoints to manage trading symbols, configure trading parameters, and interact with the Bitunix exchange. It works in conjunction with a React dashboard application (`final-bot-trader-dashboard`) for user interface interactions.

## Architecture

### Components

1. **API Server (Go)**: RESTful API service that handles business logic and database operations
2. **PostgreSQL Database**: Stores trading symbols configuration, parameters, and trading data
3. **React Dashboard**: Frontend application for visualizing and managing trading bot parameters

## Database

### Connection Details

- **Database Name**: `final_bot_trader_db`
- **Connection String**: `postgres://trader_user:trader_password@localhost:5432/go_trader_db?sslmode=disable`
- **Host**: `localhost`
- **Port**: `5432`
- **User**: `trader_user`
- **Password**: `trader_password`
- **SSL Mode**: `disable`

### Database Schema (Proposed)

The database should contain tables for:

- **Symbols**: Trading pairs configuration
  - Symbol identifier (e.g., BTC/USDT, ETH/USDT)
  - Active/Inactive status
  - Trading parameters (stop loss, take profit, etc.)
  - Configuration variables

- **Trading Parameters**: Per-symbol configuration
  - Entry conditions
  - Exit conditions
  - Risk management parameters
  - Trading strategy variables

- **Balance History**: Account balance tracking
  - Timestamp
  - Balance amount
  - Currency

## Exchange Integration

### Bitunix Exchange

- **Exchange**: Bitunix
- **Current Capabilities**: 
  - ✅ Balance retrieval (read-only)
  - ❌ Order placement (not available yet)

### API Key

The API key is configured for read-only operations. Order placement functionality will be implemented in future iterations.

## API Endpoints (Planned)

### Symbol Management

- `GET /api/v1/symbols` - List all trading symbols
- `GET /api/v1/symbols/:id` - Get symbol details
- `POST /api/v1/symbols` - Create new symbol configuration
- `PUT /api/v1/symbols/:id` - Update symbol configuration
- `PATCH /api/v1/symbols/:id/activate` - Activate a symbol
- `PATCH /api/v1/symbols/:id/deactivate` - Deactivate a symbol
- `PUT /api/v1/symbols/:id/parameters` - Update symbol trading parameters

### Balance & Account

- `GET /api/v1/balance` - Get current account balance
- `GET /api/v1/balance/history` - Get balance history

### Health & Status

- `GET /health` - Health check endpoint
- `GET /api/v1/status` - System status and exchange connectivity

## Frontend Integration

### React Dashboard

The `final-bot-trader-dashboard` application will consume the API endpoints to:

- **Visualize**:
  - List of all trading symbols and their status
  - Current account balance
  - Trading parameters for each symbol
  - Balance history charts

- **Modify**:
  - Activate/deactivate trading symbols
  - Update trading parameters per symbol
  - Configure strategy variables

## Technology Stack

- **Backend**: Go 1.21+
- **Database**: PostgreSQL
- **Frontend**: React (separate repository: `final-bot-trader-dashboard`)
- **Exchange**: Bitunix API

## Development Setup

### Prerequisites

- Go 1.21 or higher
- PostgreSQL 12+ running locally
- Database `final_bot_trader_db` created
- User `trader_user` with appropriate permissions

### Environment Variables

Create a `.env` file (or use environment variables):

```env
DB_HOST=localhost
DB_PORT=5432
DB_USER=trader_user
DB_PASSWORD=trader_password
DB_NAME=go_trader_db
DB_SSLMODE=disable

BITUNIX_API_KEY=35720dc4856673fc8d08ca7fc35bb03c
BITUNIX_SECRET_KEY=4cb70323d9cb6f8e216c3aea9cbcc4ba
BITUNIX_BASE_URL=https://fapi.bitunix.com

SERVER_PORT=8080
```

### Database Initialization

Run database migrations to create the required tables and schema.

## Security Considerations

- API keys and secrets should be stored securely (environment variables, secret management)
- Database credentials should not be hardcoded
- Implement authentication/authorization for API endpoints
- Use HTTPS in production
- Validate and sanitize all user inputs

## Future Enhancements

- [ ] Order placement functionality
- [ ] Real-time market data streaming
- [ ] WebSocket support for live updates
- [ ] Trading strategy backtesting
- [ ] Performance analytics and reporting
- [ ] Multi-exchange support
- [ ] Risk management rules engine
- [ ] Automated trading execution

## API Response Format

All API responses should follow a consistent format:

```json
{
  "success": true,
  "data": {},
  "message": "Operation successful",
  "timestamp": "2024-01-01T00:00:00Z"
}
```

Error responses:

```json
{
  "success": false,
  "error": {
    "code": "ERROR_CODE",
    "message": "Error description"
  },
  "timestamp": "2024-01-01T00:00:00Z"
}
```

## Testing

- Unit tests for business logic
- Integration tests for database operations
- API endpoint tests
- Exchange API mock for testing

## Deployment

- Build binary: `go build -o bin/api cmd/final-bot/main.go`
- Run migrations before deployment
- Configure environment variables
- Set up monitoring and logging
- Configure reverse proxy (nginx, etc.) for production
