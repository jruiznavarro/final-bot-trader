import { useEffect, useState } from 'react'
import './App.css'

interface CircuitBreaker {
  state: string
  can_trade: boolean
  consecutive_losses: number
  daily_pnl: number
  total_pnl: number
  position_multiplier: number
  pause_reason?: string
  cooldown_remaining?: string
  recovery_trades_remaining?: number
}

interface BotStatus {
  running: boolean
  dry_run: boolean
  position_size: number
  leverage: number
  open_positions: number
  closed_trades: number
  total_pnl: number
  daily_pnl: number
  daily_trades: number
  win_count: number
  loss_count: number
  win_rate: number
  timestamp: string
  circuit_breaker?: CircuitBreaker
}

interface Trade {
  id: string
  symbol: string
  side: string
  entry_price: number
  quantity: number
  stop_loss: number
  take_profit: number
  entry_time: string
  status: string
  pnl?: number
  exit_price?: number
  exit_reason?: string
  exit_time?: string
}

interface Positions {
  count: number
  positions: Trade[]
  timestamp: string
}

interface TradesResponse {
  open_positions: number
  closed_trades: number
  daily_trades: number
  total_pnl: number
  daily_pnl: number
  win_count: number
  loss_count: number
  win_rate: number
}

interface SymbolStats {
  symbol: string
  trades: number
  wins: number
  losses: number
  win_rate: number
  total_pnl: number
  avg_pnl: number
}

interface EquityPoint {
  timestamp: string
  equity: number
  trade_pnl: number
}

const API_BASE = ''

function App() {
  const [status, setStatus] = useState<BotStatus | null>(null)
  const [positions, setPositions] = useState<Positions | null>(null)
  const [trades, setTrades] = useState<TradesResponse | null>(null)
  const [closedTrades, setClosedTrades] = useState<Trade[]>([])
  const [symbolStats, setSymbolStats] = useState<SymbolStats[]>([])
  const [equityCurve, setEquityCurve] = useState<EquityPoint[]>([])
  const [error, setError] = useState<string | null>(null)
  const [lastUpdate, setLastUpdate] = useState<Date | null>(null)
  const [activeTab, setActiveTab] = useState<'overview' | 'history' | 'stats'>('overview')

  const fetchData = async () => {
    try {
      const [statusRes, positionsRes, tradesRes] = await Promise.all([
        fetch(`${API_BASE}/api/v1/bot/status`),
        fetch(`${API_BASE}/api/v1/bot/positions`),
        fetch(`${API_BASE}/api/v1/bot/trades`)
      ])

      if (!statusRes.ok || !positionsRes.ok) {
        throw new Error('Failed to fetch data')
      }

      const statusData = await statusRes.json()
      const positionsData = await positionsRes.json()
      const tradesData = await tradesRes.json()

      setStatus(statusData.data || statusData)
      setPositions(positionsData.data || positionsData)
      setTrades(tradesData.data || tradesData)
      setError(null)
      setLastUpdate(new Date())

      // Fetch additional data
      fetchClosedTrades()
      fetchSymbolStats()
      fetchEquityCurve()
    } catch {
      setError('Cannot connect to bot API. Is the bot running?')
    }
  }

  const fetchClosedTrades = async () => {
    try {
      const res = await fetch(`${API_BASE}/api/v1/bot/trades/closed`)
      if (res.ok) {
        const data = await res.json()
        setClosedTrades(data.data?.trades || data.trades || [])
      }
    } catch (e) {
      console.error('Failed to fetch closed trades:', e)
    }
  }

  const fetchSymbolStats = async () => {
    try {
      const res = await fetch(`${API_BASE}/api/v1/bot/stats/symbols`)
      if (res.ok) {
        const data = await res.json()
        const statsObj = data.data?.symbols || data.symbols || {}
        const statsArray: SymbolStats[] = Object.entries(statsObj).map(([symbol, stats]: [string, any]) => ({
          symbol,
          trades: stats.trades || 0,
          wins: stats.wins || 0,
          losses: stats.losses || 0,
          win_rate: stats.win_rate || 0,
          total_pnl: stats.total_pnl || 0,
          avg_pnl: stats.avg_pnl || 0
        }))
        setSymbolStats(statsArray.sort((a, b) => b.total_pnl - a.total_pnl))
      }
    } catch (e) {
      console.error('Failed to fetch symbol stats:', e)
    }
  }

  const fetchEquityCurve = async () => {
    try {
      const res = await fetch(`${API_BASE}/api/v1/bot/stats/equity`)
      if (res.ok) {
        const data = await res.json()
        setEquityCurve(data.data?.curve || data.curve || [])
      }
    } catch (e) {
      console.error('Failed to fetch equity curve:', e)
    }
  }

  const resetCircuitBreaker = async () => {
    try {
      const res = await fetch(`${API_BASE}/api/v1/bot/circuit-breaker/reset`, {
        method: 'POST'
      })
      if (res.ok) {
        fetchData()
      }
    } catch (e) {
      console.error('Failed to reset circuit breaker:', e)
    }
  }

  useEffect(() => {
    fetchData()
    const interval = setInterval(fetchData, 5000)
    return () => clearInterval(interval)
  }, [])

  const cb = status?.circuit_breaker

  return (
    <div className="app">
      <header className="header">
        <h1>Trading Bot Dashboard</h1>
        {lastUpdate && (
          <span className="last-update">
            Last update: {lastUpdate.toLocaleTimeString()}
          </span>
        )}
      </header>

      {error && <div className="error-banner">{error}</div>}

      {/* Navigation Tabs */}
      <div className="tab-nav">
        <button
          className={`tab-btn ${activeTab === 'overview' ? 'active' : ''}`}
          onClick={() => setActiveTab('overview')}
        >
          Overview
        </button>
        <button
          className={`tab-btn ${activeTab === 'history' ? 'active' : ''}`}
          onClick={() => setActiveTab('history')}
        >
          Trade History
        </button>
        <button
          className={`tab-btn ${activeTab === 'stats' ? 'active' : ''}`}
          onClick={() => setActiveTab('stats')}
        >
          Statistics
        </button>
      </div>

      {status && activeTab === 'overview' && (
        <div className="dashboard">
          {/* Status Row */}
          <div className="status-grid">
            <StatusCard
              title="Mode"
              value={status.dry_run ? 'DRY RUN' : 'LIVE'}
              className={status.dry_run ? 'mode-dry' : 'mode-live'}
            />
            <StatusCard
              title="Bot Status"
              value={status.running ? 'Running' : 'Stopped'}
              className={status.running ? 'status-running' : 'status-stopped'}
            />
            <StatusCard
              title="Open Positions"
              value={status.open_positions.toString()}
            />
            <StatusCard
              title="Today's Trades"
              value={status.daily_trades.toString()}
            />
          </div>

          {/* Circuit Breaker Section */}
          {cb && (
            <div className={`circuit-breaker-section cb-${cb.state.toLowerCase()}`}>
              <div className="cb-header">
                <h3>Circuit Breaker</h3>
                <div className={`cb-status-badge cb-badge-${cb.state.toLowerCase()}`}>
                  {cb.state}
                </div>
              </div>
              <div className="cb-content">
                <div className="cb-stats">
                  <div className="cb-stat">
                    <span className="cb-label">Consecutive Losses</span>
                    <span className={`cb-value ${cb.consecutive_losses >= 2 ? 'warning' : ''}`}>
                      {cb.consecutive_losses} / 3
                    </span>
                  </div>
                  <div className="cb-stat">
                    <span className="cb-label">Position Size</span>
                    <span className="cb-value">
                      {(cb.position_multiplier * 100).toFixed(0)}%
                    </span>
                  </div>
                  <div className="cb-stat">
                    <span className="cb-label">Can Trade</span>
                    <span className={`cb-value ${cb.can_trade ? 'positive' : 'negative'}`}>
                      {cb.can_trade ? 'YES' : 'NO'}
                    </span>
                  </div>
                </div>
                {cb.state === 'PAUSED' && (
                  <div className="cb-alert">
                    <span className="cb-alert-icon">⚠️</span>
                    <div className="cb-alert-content">
                      <strong>Trading Paused</strong>
                      <p>{cb.pause_reason}</p>
                      {cb.cooldown_remaining && (
                        <p className="cb-cooldown">Resumes in: {cb.cooldown_remaining}</p>
                      )}
                    </div>
                    <button className="cb-reset-btn" onClick={resetCircuitBreaker}>
                      Reset
                    </button>
                  </div>
                )}
                {cb.state === 'RECOVERY' && (
                  <div className="cb-recovery">
                    <span className="cb-recovery-icon">🔄</span>
                    <div>
                      <strong>Recovery Mode</strong>
                      <p>Trading at {(cb.position_multiplier * 100).toFixed(0)}% size</p>
                      {cb.recovery_trades_remaining !== undefined && (
                        <p>{cb.recovery_trades_remaining} winning trades to normalize</p>
                      )}
                    </div>
                  </div>
                )}
              </div>
            </div>
          )}

          {/* PnL Section */}
          <div className="pnl-section">
            <PnLCard title="Daily PnL" value={status.daily_pnl} />
            <PnLCard title="Total PnL" value={status.total_pnl} />
            <StatusCard
              title="Win Rate"
              value={`${status.win_rate.toFixed(1)}%`}
              subtitle={`${status.win_count}W / ${status.loss_count}L`}
            />
          </div>

          {/* Equity Curve */}
          {equityCurve.length > 0 && (
            <div className="equity-section">
              <h3>Equity Curve</h3>
              <EquityChart data={equityCurve} />
            </div>
          )}

          {/* Config Section */}
          <div className="config-section">
            <h3>Configuration</h3>
            <div className="config-grid">
              <div className="config-item">
                <span className="config-label">Position Size</span>
                <span className="config-value">${status.position_size}</span>
              </div>
              <div className="config-item">
                <span className="config-label">Leverage</span>
                <span className="config-value">{status.leverage}x</span>
              </div>
              <div className="config-item">
                <span className="config-label">Closed Trades</span>
                <span className="config-value">{status.closed_trades}</span>
              </div>
            </div>
          </div>
        </div>
      )}

      {/* Open Positions - Always visible in overview */}
      {activeTab === 'overview' && positions && positions.count > 0 && (
        <div className="positions-section">
          <h2>Open Positions ({positions.count})</h2>
          <div className="positions-cards">
            {positions.positions.map((pos) => (
              <PositionCard 
                key={pos.id} 
                position={pos} 
                onCloseFictitious={() => fetchData()}
              />
            ))}
          </div>
        </div>
      )}

      {activeTab === 'overview' && positions && positions.count === 0 && (
        <div className="no-positions">
          <div className="no-positions-icon">📊</div>
          <p>No open positions</p>
          <span className="no-positions-hint">Waiting for trading signals...</span>
        </div>
      )}

      {/* Trade History Tab */}
      {activeTab === 'history' && (
        <div className="history-section">
          <h2>Trade History ({closedTrades.length} trades)</h2>
          {closedTrades.length > 0 ? (
            <div className="trades-table-container">
              <table className="trades-table">
                <thead>
                  <tr>
                    <th>Date</th>
                    <th>Symbol</th>
                    <th>Side</th>
                    <th>Entry</th>
                    <th>Exit</th>
                    <th>PnL</th>
                    <th>Reason</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {closedTrades.slice(0, 50).map((trade) => (
                    <TradeRow 
                      key={trade.id} 
                      trade={trade} 
                      onUpdate={() => fetchClosedTrades()}
                    />
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <div className="no-data">
              <p>No closed trades yet</p>
            </div>
          )}
        </div>
      )}

      {/* Statistics Tab */}
      {activeTab === 'stats' && (
        <div className="stats-section">
          {/* Performance Summary */}
          {trades && (status?.win_count || 0) + (status?.loss_count || 0) > 0 && (
            <div className="performance-section">
              <h2>Performance Summary</h2>
              <div className="performance-grid">
                <div className="perf-card">
                  <div className="perf-icon win">✓</div>
                  <div className="perf-details">
                    <span className="perf-value">{status?.win_count || 0}</span>
                    <span className="perf-label">Wins</span>
                  </div>
                </div>
                <div className="perf-card">
                  <div className="perf-icon loss">✗</div>
                  <div className="perf-details">
                    <span className="perf-value">{status?.loss_count || 0}</span>
                    <span className="perf-label">Losses</span>
                  </div>
                </div>
                <div className="perf-card">
                  <div className="perf-icon neutral">⚖</div>
                  <div className="perf-details">
                    <span className="perf-value">{status?.win_rate.toFixed(1)}%</span>
                    <span className="perf-label">Win Rate</span>
                  </div>
                </div>
              </div>
            </div>
          )}

          {/* Symbol Stats */}
          <div className="symbol-stats-section">
            <h2>Performance by Symbol</h2>
            {symbolStats.length > 0 ? (
              <div className="symbol-stats-grid">
                {symbolStats.map((stat) => (
                  <div key={stat.symbol} className="symbol-stat-card">
                    <div className="symbol-stat-header">
                      <span className="symbol-name">{stat.symbol}</span>
                      <span className={`symbol-pnl ${stat.total_pnl >= 0 ? 'positive' : 'negative'}`}>
                        {stat.total_pnl >= 0 ? '+' : ''}{stat.total_pnl.toFixed(4)} USDT
                      </span>
                    </div>
                    <div className="symbol-stat-body">
                      <div className="symbol-stat-row">
                        <span>Trades</span>
                        <span>{stat.trades}</span>
                      </div>
                      <div className="symbol-stat-row">
                        <span>Win Rate</span>
                        <span className={stat.win_rate >= 50 ? 'positive' : 'negative'}>
                          {stat.win_rate.toFixed(1)}%
                        </span>
                      </div>
                      <div className="symbol-stat-row">
                        <span>Avg PnL</span>
                        <span className={stat.avg_pnl >= 0 ? 'positive' : 'negative'}>
                          {stat.avg_pnl >= 0 ? '+' : ''}{stat.avg_pnl.toFixed(4)}
                        </span>
                      </div>
                      <div className="symbol-stat-bar">
                        <div
                          className="symbol-stat-bar-fill"
                          style={{ width: `${stat.win_rate}%` }}
                        />
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            ) : (
              <div className="no-data">
                <p>No symbol statistics yet</p>
              </div>
            )}
          </div>

          {/* Equity Curve in Stats */}
          {equityCurve.length > 0 && (
            <div className="equity-section">
              <h2>Equity Curve</h2>
              <EquityChart data={equityCurve} />
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function StatusCard({ title, value, className = '', subtitle = '' }: {
  title: string
  value: string
  className?: string
  subtitle?: string
}) {
  return (
    <div className={`status-card ${className}`}>
      <div className="card-title">{title}</div>
      <div className="card-value">{value}</div>
      {subtitle && <div className="card-subtitle">{subtitle}</div>}
    </div>
  )
}

function PnLCard({ title, value }: { title: string; value: number }) {
  const isPositive = value >= 0
  return (
    <div className={`pnl-card ${isPositive ? 'pnl-positive' : 'pnl-negative'}`}>
      <div className="card-title">{title}</div>
      <div className="card-value">
        {isPositive ? '+' : ''}{value.toFixed(4)} USDT
      </div>
    </div>
  )
}

function PositionCard({ position, onCloseFictitious }: { position: Trade; onCloseFictitious: (id: string) => void }) {
  const isLong = position.side === 'LONG'
  const riskPct = isLong
    ? ((position.entry_price - position.stop_loss) / position.entry_price * 100)
    : ((position.stop_loss - position.entry_price) / position.entry_price * 100)
  const rewardPct = isLong
    ? ((position.take_profit - position.entry_price) / position.entry_price * 100)
    : ((position.entry_price - position.take_profit) / position.entry_price * 100)
  const rr = rewardPct / riskPct
  const [isClosing, setIsClosing] = useState(false)

  const handleCloseFictitious = async () => {
    if (!confirm('¿Cerrar esta posición ficticiamente? Esto solo actualizará la base de datos, NO cerrará la posición en Bitunix.')) {
      return
    }

    setIsClosing(true)
    try {
      const res = await fetch(`${API_BASE}/api/v1/bot/positions/${position.id}/close-fictitious`, {
        method: 'POST'
      })
      if (res.ok) {
        onCloseFictitious(position.id)
      } else {
        const data = await res.json()
        alert(`Error: ${data.error || 'Failed to close position'}`)
      }
    } catch (e) {
      alert(`Error: ${e instanceof Error ? e.message : 'Failed to close position'}`)
    } finally {
      setIsClosing(false)
    }
  }

  return (
    <div className={`position-card ${isLong ? 'position-long' : 'position-short'}`}>
      <div className="position-header">
        <span className="position-symbol">{position.symbol}</span>
        <span className={`position-side ${isLong ? 'side-long' : 'side-short'}`}>
          {isLong ? '📈 LONG' : '📉 SHORT'}
        </span>
      </div>
      <div className="position-body">
        <div className="position-row">
          <span className="position-label">Entry</span>
          <span className="position-value">{position.entry_price.toFixed(6)}</span>
        </div>
        <div className="position-row">
          <span className="position-label">Quantity</span>
          <span className="position-value">{position.quantity.toFixed(4)}</span>
        </div>
        <div className="position-row">
          <span className="position-label sl">Stop Loss</span>
          <span className="position-value sl">{position.stop_loss.toFixed(6)} (-{riskPct.toFixed(1)}%)</span>
        </div>
        <div className="position-row">
          <span className="position-label tp">Take Profit</span>
          <span className="position-value tp">{position.take_profit.toFixed(6)} (+{rewardPct.toFixed(1)}%)</span>
        </div>
        <div className="position-row">
          <span className="position-label">R:R Ratio</span>
          <span className="position-value rr">1:{rr.toFixed(1)}</span>
        </div>
      </div>
      <div className="position-footer">
        <span className="position-time">
          {new Date(position.entry_time).toLocaleString()}
        </span>
        <button
          className="close-fictitious-btn"
          onClick={handleCloseFictitious}
          disabled={isClosing}
          title="Cerrar posición ficticiamente (solo en BD, no en Bitunix)"
        >
          {isClosing ? 'Cerrando...' : '🗑️ Cerrar Ficticio'}
        </button>
      </div>
    </div>
  )
}

function TradeRow({ trade, onUpdate }: { trade: Trade; onUpdate: () => void }) {
  const [isEditing, setIsEditing] = useState(false)
  const [entryPrice, setEntryPrice] = useState(trade.entry_price.toString())
  const [exitPrice, setExitPrice] = useState(trade.exit_price?.toString() || '')
  const [isSaving, setIsSaving] = useState(false)
  const [calculatedPnL, setCalculatedPnL] = useState<number | null>(null)

  const calculatePnL = (entry: number, exit: number) => {
    if (!exit || isNaN(entry) || isNaN(exit)) return null
    const isLong = trade.side === 'LONG'
    if (isLong) {
      return (exit - entry) * trade.quantity
    } else {
      return (entry - exit) * trade.quantity
    }
  }

  const handleEdit = () => {
    setIsEditing(true)
    setEntryPrice(trade.entry_price.toString())
    setExitPrice(trade.exit_price?.toString() || '')
    setCalculatedPnL(null)
  }

  const handleCancel = () => {
    setIsEditing(false)
    setEntryPrice(trade.entry_price.toString())
    setExitPrice(trade.exit_price?.toString() || '')
    setCalculatedPnL(null)
  }

  const handleSave = async () => {
    const entry = parseFloat(entryPrice)
    const exit = exitPrice ? parseFloat(exitPrice) : null

    if (isNaN(entry) || (exitPrice && (isNaN(exit!) || exit! <= 0))) {
      alert('Please enter valid prices')
      return
    }

    setIsSaving(true)
    try {
      const body: any = {}
      if (entry !== trade.entry_price) {
        body.entry_price = entry
      }
      if (exit !== null && exit !== trade.exit_price) {
        body.exit_price = exit
      }

      if (Object.keys(body).length === 0) {
        setIsEditing(false)
        return
      }

      const res = await fetch(`${API_BASE}/api/v1/bot/trades/${trade.id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body)
      })

      if (res.ok) {
        setIsEditing(false)
        onUpdate()
      } else {
        const data = await res.json()
        alert(`Error: ${data.error || 'Failed to update trade'}`)
      }
    } catch (e) {
      alert(`Error: ${e instanceof Error ? e.message : 'Failed to update trade'}`)
    } finally {
      setIsSaving(false)
    }
  }

  const handlePriceChange = (field: 'entry' | 'exit', value: string) => {
    if (field === 'entry') {
      setEntryPrice(value)
    } else {
      setExitPrice(value)
    }

    // Recalculate PnL
    const entry = parseFloat(field === 'entry' ? value : entryPrice)
    const exit = parseFloat(field === 'exit' ? value : exitPrice)
    if (!isNaN(entry) && !isNaN(exit) && exit > 0) {
      const pnl = calculatePnL(entry, exit)
      setCalculatedPnL(pnl)
    } else {
      setCalculatedPnL(null)
    }
  }

  const currentPnL = calculatedPnL !== null ? calculatedPnL : trade.pnl
  const isLong = trade.side === 'LONG'

  return (
    <tr className={currentPnL && currentPnL >= 0 ? 'trade-win' : 'trade-loss'}>
      <td>{new Date(trade.exit_time || trade.entry_time).toLocaleDateString()}</td>
      <td className="symbol-cell">{trade.symbol}</td>
      <td className={isLong ? 'side-long' : 'side-short'}>
        {trade.side}
      </td>
      <td>
        {isEditing ? (
          <input
            type="number"
            step="0.0001"
            value={entryPrice}
            onChange={(e) => handlePriceChange('entry', e.target.value)}
            className="price-input"
            disabled={isSaving}
          />
        ) : (
          trade.entry_price.toFixed(4)
        )}
      </td>
      <td>
        {isEditing ? (
          <input
            type="number"
            step="0.0001"
            value={exitPrice}
            onChange={(e) => handlePriceChange('exit', e.target.value)}
            className="price-input"
            disabled={isSaving}
          />
        ) : (
          trade.exit_price?.toFixed(4) || '-'
        )}
      </td>
      <td className={currentPnL && currentPnL >= 0 ? 'pnl-positive' : 'pnl-negative'}>
        {currentPnL !== null && currentPnL !== undefined
          ? `${currentPnL >= 0 ? '+' : ''}${currentPnL.toFixed(4)}`
          : '-'}
        {isEditing && calculatedPnL !== null && calculatedPnL !== trade.pnl && (
          <span className="pnl-preview"> (preview)</span>
        )}
      </td>
      <td className="reason-cell">{trade.exit_reason || '-'}</td>
      <td className="trade-actions">
        {isEditing ? (
          <>
            <button
              className="save-btn"
              onClick={handleSave}
              disabled={isSaving}
              title="Save changes"
            >
              {isSaving ? 'Saving...' : '✓'}
            </button>
            <button
              className="cancel-btn"
              onClick={handleCancel}
              disabled={isSaving}
              title="Cancel editing"
            >
              ✗
            </button>
          </>
        ) : (
          <button
            className="edit-btn"
            onClick={handleEdit}
            title="Edit prices"
          >
            ✏️
          </button>
        )}
      </td>
    </tr>
  )
}

function EquityChart({ data }: { data: EquityPoint[] }) {
  if (data.length < 2) {
    return <div className="no-data"><p>Not enough data for chart</p></div>
  }

  const width = 800
  const height = 200
  const padding = { top: 20, right: 20, bottom: 30, left: 60 }
  const chartWidth = width - padding.left - padding.right
  const chartHeight = height - padding.top - padding.bottom

  const equities = data.map(d => d.equity)
  const minEquity = Math.min(...equities)
  const maxEquity = Math.max(...equities)
  const equityRange = maxEquity - minEquity || 1

  const xScale = (i: number) => padding.left + (i / (data.length - 1)) * chartWidth
  const yScale = (v: number) => padding.top + chartHeight - ((v - minEquity) / equityRange) * chartHeight

  const pathD = data.map((d, i) => `${i === 0 ? 'M' : 'L'} ${xScale(i)} ${yScale(d.equity)}`).join(' ')

  const areaD = `${pathD} L ${xScale(data.length - 1)} ${height - padding.bottom} L ${padding.left} ${height - padding.bottom} Z`

  const isPositive = data[data.length - 1].equity >= data[0].equity
  const strokeColor = isPositive ? '#00c853' : '#ff5252'
  const fillColor = isPositive ? 'rgba(0, 200, 83, 0.1)' : 'rgba(255, 82, 82, 0.1)'

  return (
    <div className="equity-chart">
      <svg viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="xMidYMid meet">
        {/* Grid lines */}
        {[0, 0.25, 0.5, 0.75, 1].map((pct) => (
          <g key={pct}>
            <line
              x1={padding.left}
              y1={padding.top + chartHeight * (1 - pct)}
              x2={width - padding.right}
              y2={padding.top + chartHeight * (1 - pct)}
              stroke="#333"
              strokeDasharray="4"
            />
            <text
              x={padding.left - 10}
              y={padding.top + chartHeight * (1 - pct) + 4}
              fill="#666"
              fontSize="10"
              textAnchor="end"
            >
              {(minEquity + equityRange * pct).toFixed(2)}
            </text>
          </g>
        ))}

        {/* Area fill */}
        <path d={areaD} fill={fillColor} />

        {/* Line */}
        <path d={pathD} fill="none" stroke={strokeColor} strokeWidth="2" />

        {/* Data points */}
        {data.map((d, i) => (
          <circle
            key={i}
            cx={xScale(i)}
            cy={yScale(d.equity)}
            r="3"
            fill={strokeColor}
          />
        ))}
      </svg>
      <div className="equity-summary">
        <span>Start: {data[0].equity.toFixed(4)} USDT</span>
        <span>Current: {data[data.length - 1].equity.toFixed(4)} USDT</span>
        <span className={isPositive ? 'positive' : 'negative'}>
          Change: {isPositive ? '+' : ''}{(data[data.length - 1].equity - data[0].equity).toFixed(4)} USDT
        </span>
      </div>
    </div>
  )
}

export default App
