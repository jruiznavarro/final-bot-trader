package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

const (
	botName     = "🤖 Trading Bot"
	telegramAPI = "https://api.telegram.org/bot%s/sendMessage"
)

// Client handles Telegram notifications
type Client struct {
	token  string
	chatID string
	client *http.Client
}

// NewClient creates a new Telegram client
func NewClient(token, chatID string) *Client {
	return &Client{
		token:  token,
		chatID: chatID,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

// IsConfigured returns true if Telegram is properly configured
func (c *Client) IsConfigured() bool {
	return c.token != "" && c.chatID != ""
}

// SendMessage sends a message to the configured chat
func (c *Client) SendMessage(message string) error {
	if !c.IsConfigured() {
		return nil // Silently skip if not configured
	}

	url := fmt.Sprintf(telegramAPI, c.token)

	payload := map[string]interface{}{
		"chat_id":    c.chatID,
		"text":       message,
		"parse_mode": "HTML",
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}

	resp, err := c.client.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("send error: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram API error: status %d", resp.StatusCode)
	}

	return nil
}

// NotifyStartup sends a startup notification
func (c *Client) NotifyStartup(mode string, positionSizePct float64, leverage int, symbols []string, primaryInterval, entryInterval string, maxDailyLoss float64, maxOpenPositions int) error {
	symbolList := ""
	for _, s := range symbols {
		symbolList += "• " + s + "\n"
	}

	msg := fmt.Sprintf(`<b>%s</b>

✅ <b>Bot Iniciado</b>

📊 <b>Configuración:</b>
• Modo: <b>%s</b>
• Tamaño posición: <code>%.0f%% del balance</code>
• Apalancamiento: <code>%dx</code>
• Estrategia: <code>MTF %s + %s</code>
• Máx. posiciones: <code>%d</code>
• Límite pérdida diaria: <code>$%.0f</code>

📋 <b>Símbolos (%d):</b>
%s
Monitoreando señales...`, botName, mode, positionSizePct*100, leverage, primaryInterval, entryInterval, maxOpenPositions, maxDailyLoss, len(symbols), symbolList)

	return c.SendMessage(msg)
}

// NotifyPositionOpened sends notification when a position is opened
func (c *Client) NotifyPositionOpened(symbol, side string, entryPrice, quantity, tp, sl float64, reason string) error {
	emoji := "🟢"
	direction := "📈 LONG"
	if side == "SHORT" {
		emoji = "🔴"
		direction = "📉 SHORT"
	}

	// Calculate risk/reward
	var riskPct, rewardPct float64
	if side == "LONG" {
		riskPct = (entryPrice - sl) / entryPrice * 100
		rewardPct = (tp - entryPrice) / entryPrice * 100
	} else {
		riskPct = (sl - entryPrice) / entryPrice * 100
		rewardPct = (entryPrice - tp) / entryPrice * 100
	}
	rr := rewardPct / riskPct

	// Calculate position value
	positionValue := entryPrice * quantity

	msg := fmt.Sprintf(`%s <b>NUEVA POSICIÓN</b>

<b>%s</b> %s
━━━━━━━━━━━━━━━━━━━━━

💵 <b>Entrada:</b> <code>%.6f</code>
📦 <b>Cantidad:</b> <code>%.4f</code>
💰 <b>Valor:</b> <code>$%.2f</code>

🎯 <b>Take Profit:</b> <code>%.6f</code> (+%.1f%%)
🛑 <b>Stop Loss:</b> <code>%.6f</code> (-%.1f%%)
⚖️ <b>R:R Ratio:</b> <code>1:%.1f</code>

━━━━━━━━━━━━━━━━━━━━━
📊 %s`, emoji, symbol, direction, entryPrice, quantity, positionValue, tp, rewardPct, sl, riskPct, rr, reason)

	return c.SendMessage(msg)
}

// NotifyPositionClosed sends notification when a position is closed
func (c *Client) NotifyPositionClosed(symbol, side, exitReason string, entryPrice, exitPrice, pnl float64) error {
	emoji := "✅"
	pnlEmoji := "💰"
	if pnl < 0 {
		emoji = "❌"
		pnlEmoji = "📉"
	}

	msg := fmt.Sprintf(`<b>%s</b>

%s <b>POSICIÓN CERRADA</b>

📈 <b>%s</b> - %s
━━━━━━━━━━━━━━━━━━━━━━━
• Precio entrada: <code>%.6f</code>
• Precio salida: <code>%.6f</code>
%s PnL: <code>%+.4f USDT</code>
━━━━━━━━━━━━━━━━━━━━━━━
📝 Motivo: %s`, botName, emoji, symbol, side, entryPrice, exitPrice, pnlEmoji, pnl, exitReason)

	return c.SendMessage(msg)
}

// NotifyTPHit sends notification when take profit is hit
func (c *Client) NotifyTPHit(symbol, side string, entryPrice, tpPrice, pnl float64) error {
	// Calculate percentage
	var pnlPct float64
	if side == "LONG" {
		pnlPct = (tpPrice - entryPrice) / entryPrice * 100
	} else {
		pnlPct = (entryPrice - tpPrice) / entryPrice * 100
	}

	msg := fmt.Sprintf(`🎯 <b>¡TAKE PROFIT!</b> ✅

<b>%s</b> - %s
━━━━━━━━━━━━━━━━━━━━━

📥 <b>Entrada:</b> <code>%.6f</code>
📤 <b>Salida:</b> <code>%.6f</code>

💰 <b>Ganancia:</b> <code>%+.4f USDT</code>
📈 <b>Retorno:</b> <code>%+.2f%%</code>

<i>¡Objetivo alcanzado! 🚀</i>`, symbol, side, entryPrice, tpPrice, pnl, pnlPct)

	return c.SendMessage(msg)
}

// NotifySLHit sends notification when stop loss is hit
func (c *Client) NotifySLHit(symbol, side string, entryPrice, slPrice, pnl float64) error {
	// Calculate percentage
	var pnlPct float64
	if side == "LONG" {
		pnlPct = (slPrice - entryPrice) / entryPrice * 100
	} else {
		pnlPct = (entryPrice - slPrice) / entryPrice * 100
	}

	msg := fmt.Sprintf(`🛑 <b>STOP LOSS</b> ❌

<b>%s</b> - %s
━━━━━━━━━━━━━━━━━━━━━

📥 <b>Entrada:</b> <code>%.6f</code>
📤 <b>Salida:</b> <code>%.6f</code>

📉 <b>Pérdida:</b> <code>%+.4f USDT</code>
📊 <b>Retorno:</b> <code>%.2f%%</code>

<i>Riesgo controlado ✓</i>`, symbol, side, entryPrice, slPrice, pnl, pnlPct)

	return c.SendMessage(msg)
}

// NotifyError sends an error notification
func (c *Client) NotifyError(context string, err error) error {
	msg := fmt.Sprintf(`<b>%s</b>

⚠️ <b>ERROR</b>

📍 Contexto: %s
❌ Error: <code>%v</code>`, botName, context, err)

	return c.SendMessage(msg)
}

// NotifyDailySummary sends a daily summary
func (c *Client) NotifyDailySummary(totalTrades, wins, losses int, totalPnL, winRate float64) error {
	emoji := "📈"
	status := "POSITIVO"
	if totalPnL < 0 {
		emoji = "📉"
		status = "NEGATIVO"
	}

	msg := fmt.Sprintf(`📊 <b>RESUMEN DIARIO</b>

Día %s %s
━━━━━━━━━━━━━━━━━━━━━

📈 <b>Trades:</b> <code>%d</code>
✅ <b>Ganadas:</b> <code>%d</code>
❌ <b>Perdidas:</b> <code>%d</code>

🎯 <b>Win Rate:</b> <code>%.1f%%</code>
%s <b>PnL:</b> <code>%+.4f USDT</code>

━━━━━━━━━━━━━━━━━━━━━
<i>Generado automáticamente</i>`, status, emoji, totalTrades, wins, losses, winRate, emoji, totalPnL)

	return c.SendMessage(msg)
}

// NotifyStatus sends a status update
func (c *Client) NotifyStatus(openPositions, closedTrades int, dailyPnL, totalPnL, winRate float64) error {
	dailyEmoji := "🟢"
	if dailyPnL < 0 {
		dailyEmoji = "🔴"
	}
	totalEmoji := "📈"
	if totalPnL < 0 {
		totalEmoji = "📉"
	}

	msg := fmt.Sprintf(`📋 <b>ESTADO DEL BOT</b>

━━━━━━━━━━━━━━━━━━━━━
📊 <b>Posiciones abiertas:</b> <code>%d</code>
📈 <b>Trades cerrados:</b> <code>%d</code>

%s <b>PnL Hoy:</b> <code>%+.4f USDT</code>
%s <b>PnL Total:</b> <code>%+.4f USDT</code>

🎯 <b>Win Rate:</b> <code>%.1f%%</code>
━━━━━━━━━━━━━━━━━━━━━`, openPositions, closedTrades, dailyEmoji, dailyPnL, totalEmoji, totalPnL, winRate)

	return c.SendMessage(msg)
}

// NotifyTrailingStopHit sends notification when trailing stop is hit
func (c *Client) NotifyTrailingStopHit(symbol, side string, entryPrice, exitPrice, pnl, pnlPct float64) error {
	msg := fmt.Sprintf(`🎯 <b>TRAILING STOP</b> 💰

<b>%s</b> - %s
━━━━━━━━━━━━━━━━━━━━━

📥 <b>Entrada:</b> <code>%.6f</code>
📤 <b>Salida:</b> <code>%.6f</code>

💰 <b>Ganancia:</b> <code>%+.4f USDT</code>
📈 <b>Retorno:</b> <code>%+.2f%%</code>

<i>¡Ganancias protegidas! 🔒</i>`, symbol, side, entryPrice, exitPrice, pnl, pnlPct)

	return c.SendMessage(msg)
}

// NotifyShutdown sends a shutdown notification
func (c *Client) NotifyShutdown(openPositions int, totalPnL float64) error {
	msg := fmt.Sprintf(`<b>%s</b>

🔴 <b>Bot Detenido</b>

📊 Posiciones abiertas: %d
💰 PnL total: <code>%+.4f USDT</code>

⚠️ Las posiciones abiertas mantienen sus TP/SL en el exchange.`, botName, openPositions, totalPnL)

	return c.SendMessage(msg)
}
