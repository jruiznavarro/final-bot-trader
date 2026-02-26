# ETF Trading Bot

Bot de trading automatizado para ETFs usando Alpaca Markets.

## Requisitos

1. Cuenta en [Alpaca Markets](https://alpaca.markets) (gratis)
2. Go 1.23+

## Configuracion

1. Crea un archivo `.env` con tus credenciales de Alpaca:

```bash
cp .env.example .env
```

2. Edita `.env` con tus API keys:

```
ALPACA_API_KEY=tu_api_key
ALPACA_API_SECRET=tu_api_secret
```

Las API keys las encuentras en: https://app.alpaca.markets/paper/dashboard/overview
(Click en "API Keys" en el menu lateral)

## Uso

### Paper Trading (dinero ficticio)

```bash
go run ./cmd/paper-trading
```

### Backtest

```bash
go run ./cmd/backtest
```

## ETFs soportados

- **SPY** - S&P 500 ETF
- **QQQ** - Nasdaq 100 ETF
- **IWM** - Russell 2000 ETF

## Estrategia

El bot usa una estrategia de **momentum multi-timeframe**:

1. **Daily (1d)**: Determina la tendencia principal
2. **Hourly (1h)**: Busca entradas alineadas con la tendencia

### Condiciones de entrada (LONG):
- Fast EMA > Slow EMA
- Precio > Fast EMA
- RSI entre 40-70
- Momentum positivo
- Estructura de higher lows

### Gestion de riesgo:
- Stop Loss: 1.5x ATR
- Take Profit: 2.5x ATR (R:R = 1.67)
- Posicion: 5% del equity por trade
- Max 2 posiciones simultaneas

## Estructura

```
etf-bot-trader-api/
├── cmd/
│   ├── paper-trading/  # Trading con dinero ficticio
│   ├── backtest/       # Backtesting historico
│   └── live-trading/   # Trading real (futuro)
├── internal/
│   ├── exchange/       # Cliente Alpaca API
│   ├── strategy/       # Estrategia de trading
│   ├── livetrading/    # Motor de trading
│   ├── backtest/       # Motor de backtest
│   └── indicator/      # Indicadores tecnicos
└── data/               # Estado persistente
```

## Notas

- El bot solo opera durante horario de mercado (9:30-16:00 ET)
- Usa paper trading para validar antes de dinero real
- Las ordenes incluyen TP y SL automaticos (bracket orders)
