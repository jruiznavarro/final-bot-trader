package database

import (
	"context"
	"database/sql"
	"time"
)

// Trade represents a trade record in the database
type Trade struct {
	ID         string
	Symbol     string
	Side       string
	EntryPrice float64
	ExitPrice  sql.NullFloat64
	Quantity   float64
	StopLoss   sql.NullFloat64
	TakeProfit sql.NullFloat64
	PnL        sql.NullFloat64
	Status     string
	Reason     sql.NullString
	ExitReason sql.NullString
	OrderID    sql.NullInt64
	PositionID sql.NullString
	EntryTime  time.Time
	ExitTime   sql.NullTime
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// TradeRepository handles trade database operations
type TradeRepository struct {
	db *DB
}

// NewTradeRepository creates a new trade repository
func NewTradeRepository(db *DB) *TradeRepository {
	return &TradeRepository{db: db}
}

// Create inserts a new trade
func (r *TradeRepository) Create(ctx context.Context, trade *Trade) error {
	query := `
		INSERT INTO trades (
			id, symbol, side, entry_price, quantity, stop_loss, take_profit,
			status, reason, order_id, position_id, entry_time
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	_, err := r.db.ExecContext(ctx, query,
		trade.ID,
		trade.Symbol,
		trade.Side,
		trade.EntryPrice,
		trade.Quantity,
		trade.StopLoss,
		trade.TakeProfit,
		trade.Status,
		trade.Reason,
		trade.OrderID,
		trade.PositionID,
		trade.EntryTime,
	)

	return err
}

// Update updates an existing trade
func (r *TradeRepository) Update(ctx context.Context, trade *Trade) error {
	query := `
		UPDATE trades SET
			entry_price = $2,
			exit_price = $3,
			pnl = $4,
			status = $5,
			exit_reason = $6,
			exit_time = $7,
			position_id = $8,
			updated_at = CURRENT_TIMESTAMP
		WHERE id = $1
	`

	_, err := r.db.ExecContext(ctx, query,
		trade.ID,
		trade.EntryPrice,
		trade.ExitPrice,
		trade.PnL,
		trade.Status,
		trade.ExitReason,
		trade.ExitTime,
		trade.PositionID,
	)

	return err
}

// GetByID retrieves a trade by ID
func (r *TradeRepository) GetByID(ctx context.Context, id string) (*Trade, error) {
	query := `
		SELECT id, symbol, side, entry_price, exit_price, quantity, stop_loss, take_profit,
			   pnl, status, reason, exit_reason, order_id, position_id, entry_time, exit_time,
			   created_at, updated_at
		FROM trades WHERE id = $1
	`

	trade := &Trade{}
	err := r.db.QueryRowContext(ctx, query, id).Scan(
		&trade.ID, &trade.Symbol, &trade.Side, &trade.EntryPrice, &trade.ExitPrice,
		&trade.Quantity, &trade.StopLoss, &trade.TakeProfit, &trade.PnL, &trade.Status,
		&trade.Reason, &trade.ExitReason, &trade.OrderID, &trade.PositionID,
		&trade.EntryTime, &trade.ExitTime, &trade.CreatedAt, &trade.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return trade, nil
}

// GetOpenTrades retrieves all open trades
func (r *TradeRepository) GetOpenTrades(ctx context.Context) ([]*Trade, error) {
	query := `
		SELECT id, symbol, side, entry_price, exit_price, quantity, stop_loss, take_profit,
			   pnl, status, reason, exit_reason, order_id, position_id, entry_time, exit_time,
			   created_at, updated_at
		FROM trades WHERE status = 'OPEN'
		ORDER BY entry_time DESC
	`

	return r.queryTrades(ctx, query)
}

// GetTradesByDateRange retrieves trades within a date range
func (r *TradeRepository) GetTradesByDateRange(ctx context.Context, start, end time.Time) ([]*Trade, error) {
	query := `
		SELECT id, symbol, side, entry_price, exit_price, quantity, stop_loss, take_profit,
			   pnl, status, reason, exit_reason, order_id, position_id, entry_time, exit_time,
			   created_at, updated_at
		FROM trades
		WHERE entry_time >= $1 AND entry_time < $2
		ORDER BY entry_time DESC
	`

	return r.queryTradesWithArgs(ctx, query, start, end)
}

// GetRecentTrades retrieves the most recent trades
func (r *TradeRepository) GetRecentTrades(ctx context.Context, limit int) ([]*Trade, error) {
	query := `
		SELECT id, symbol, side, entry_price, exit_price, quantity, stop_loss, take_profit,
			   pnl, status, reason, exit_reason, order_id, position_id, entry_time, exit_time,
			   created_at, updated_at
		FROM trades
		ORDER BY entry_time DESC
		LIMIT $1
	`

	return r.queryTradesWithArgs(ctx, query, limit)
}

// GetStats retrieves overall trading statistics
func (r *TradeRepository) GetStats(ctx context.Context) (map[string]interface{}, error) {
	query := `
		SELECT
			COUNT(*) as total_trades,
			COUNT(CASE WHEN status = 'OPEN' THEN 1 END) as open_trades,
			COUNT(CASE WHEN status = 'CLOSED' THEN 1 END) as closed_trades,
			COUNT(CASE WHEN pnl > 0 THEN 1 END) as wins,
			COUNT(CASE WHEN pnl < 0 THEN 1 END) as losses,
			COALESCE(SUM(pnl), 0) as total_pnl,
			COALESCE(AVG(CASE WHEN pnl > 0 THEN pnl END), 0) as avg_win,
			COALESCE(AVG(CASE WHEN pnl < 0 THEN pnl END), 0) as avg_loss
		FROM trades
	`

	var totalTrades, openTrades, closedTrades, wins, losses int
	var totalPnL, avgWin, avgLoss float64

	err := r.db.QueryRowContext(ctx, query).Scan(
		&totalTrades, &openTrades, &closedTrades, &wins, &losses,
		&totalPnL, &avgWin, &avgLoss,
	)
	if err != nil {
		return nil, err
	}

	winRate := 0.0
	if wins+losses > 0 {
		winRate = float64(wins) / float64(wins+losses) * 100
	}

	return map[string]interface{}{
		"total_trades":  totalTrades,
		"open_trades":   openTrades,
		"closed_trades": closedTrades,
		"wins":          wins,
		"losses":        losses,
		"total_pnl":     totalPnL,
		"avg_win":       avgWin,
		"avg_loss":      avgLoss,
		"win_rate":      winRate,
	}, nil
}

// GetDailyStats retrieves stats for a specific date
func (r *TradeRepository) GetDailyStats(ctx context.Context, date time.Time) (map[string]interface{}, error) {
	startOfDay := date.Truncate(24 * time.Hour)
	endOfDay := startOfDay.Add(24 * time.Hour)

	query := `
		SELECT
			COUNT(*) as total_trades,
			COUNT(CASE WHEN pnl > 0 THEN 1 END) as wins,
			COUNT(CASE WHEN pnl < 0 THEN 1 END) as losses,
			COALESCE(SUM(pnl), 0) as total_pnl
		FROM trades
		WHERE entry_time >= $1 AND entry_time < $2
	`

	var totalTrades, wins, losses int
	var totalPnL float64

	err := r.db.QueryRowContext(ctx, query, startOfDay, endOfDay).Scan(
		&totalTrades, &wins, &losses, &totalPnL,
	)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"date":         startOfDay.Format("2006-01-02"),
		"total_trades": totalTrades,
		"wins":         wins,
		"losses":       losses,
		"total_pnl":    totalPnL,
	}, nil
}

func (r *TradeRepository) queryTrades(ctx context.Context, query string) ([]*Trade, error) {
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanTrades(rows)
}

func (r *TradeRepository) queryTradesWithArgs(ctx context.Context, query string, args ...interface{}) ([]*Trade, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanTrades(rows)
}

func (r *TradeRepository) scanTrades(rows *sql.Rows) ([]*Trade, error) {
	var trades []*Trade

	for rows.Next() {
		trade := &Trade{}
		err := rows.Scan(
			&trade.ID, &trade.Symbol, &trade.Side, &trade.EntryPrice, &trade.ExitPrice,
			&trade.Quantity, &trade.StopLoss, &trade.TakeProfit, &trade.PnL, &trade.Status,
			&trade.Reason, &trade.ExitReason, &trade.OrderID, &trade.PositionID,
			&trade.EntryTime, &trade.ExitTime, &trade.CreatedAt, &trade.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		trades = append(trades, trade)
	}

	return trades, rows.Err()
}
