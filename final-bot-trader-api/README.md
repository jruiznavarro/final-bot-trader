# Final Bot Trader API

API para el sistema de trading automatizado.

## Requisitos

- Go 1.21 o superior

## Instalación

```bash
go mod download
```

## Ejecución

```bash
go run cmd/final-bot/main.go
```

El servidor se iniciará en `http://localhost:8080`

## Endpoints

- `GET /` - Endpoint principal
- `GET /health` - Health check

## Desarrollo

```bash
# Ejecutar en modo desarrollo
go run cmd/final-bot/main.go

# Compilar
go build -o bin/api cmd/final-bot/main.go

# Ejecutar binario compilado
./bin/api
```
