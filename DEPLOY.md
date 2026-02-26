# Deployment Guide - Copy Trading Bot

## Prerequisites

- Docker & Docker Compose installed
- Git installed

## Quick Deploy

### 1. Clone the repository to your server

```bash
git clone <your-repo-url> /opt/trading-bot
cd /opt/trading-bot
```

### 2. Create environment file

```bash
cp .env.example .env
nano .env
```

Fill in your credentials:
```
BITUNIX_API_KEY=your_api_key
BITUNIX_SECRET_KEY=your_secret_key
TELEGRAM_BOT_TOKEN=your_telegram_token
TELEGRAM_CHAT_ID=your_chat_id
```

### 3. Build and start

```bash
docker-compose up -d --build
```

### 4. Check status

```bash
# View logs
docker-compose logs -f bot

# Check all containers
docker-compose ps
```

## Access

- **Dashboard**: http://your-server-ip:3000
- **API**: http://your-server-ip:8080/health

## Management Commands

```bash
# Stop all
docker-compose down

# Restart bot only
docker-compose restart bot

# View bot logs
docker-compose logs -f bot

# View all logs
docker-compose logs -f

# Rebuild after code changes
docker-compose up -d --build

# Check database
docker exec -it trading-db psql -U trader -d trading_bot
```

## Ports Used

| Service | Port | Description |
|---------|------|-------------|
| Dashboard | 3000 | Web interface |
| Bot API | 8080 | REST API |
| PostgreSQL | 5432 | Database (internal only) |

## Data Persistence

Data is stored in Docker volumes:
- `postgres_data`: Database
- `bot_data`: Bot state files

To backup:
```bash
docker run --rm -v trading-bot_postgres_data:/data -v $(pwd):/backup alpine tar czf /backup/db-backup.tar.gz /data
```

## Update Bot

```bash
cd /opt/trading-bot
git pull
docker-compose up -d --build
```

## Troubleshooting

### Bot not starting
```bash
docker-compose logs bot
```

### Database connection issues
```bash
docker-compose logs postgres
docker exec -it trading-db pg_isready -U trader
```

### Dashboard not loading
```bash
docker-compose logs dashboard
curl http://localhost:3000
```
