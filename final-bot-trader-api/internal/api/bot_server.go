package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"final-bot-trader-api/internal/api/handlers"
	"final-bot-trader-api/internal/livetrading"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// BotServer represents the REST API server for the trading bot
type BotServer struct {
	engine *livetrading.Engine
	server *http.Server
	router *chi.Mux
}

// NewBotServer creates a new bot API server
func NewBotServer(engine *livetrading.Engine, port string) *BotServer {
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// CORS
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	})

	s := &BotServer{
		engine: engine,
		router: r,
		server: &http.Server{
			Addr:         ":" + port,
			Handler:      r,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		},
	}

	s.registerRoutes()

	return s
}

func (s *BotServer) registerRoutes() {
	h := handlers.NewBotHandler(s.engine)

	// Health check
	s.router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		handlers.JSON(w, http.StatusOK, map[string]interface{}{
			"status":    "ok",
			"service":   "copy-trading-bot",
			"timestamp": time.Now().Format(time.RFC3339),
		})
	})

	// Bot API routes
	s.router.Route("/api/v1/bot", func(r chi.Router) {
		r.Get("/status", h.GetBotStatus)
		r.Get("/positions", h.GetBotPositions)
		r.Get("/trades", h.GetBotTrades)
		r.Get("/config", h.GetBotConfig)
		r.Get("/circuit-breaker", h.GetCircuitBreakerStatus)
		r.Post("/circuit-breaker/reset", h.ResetCircuitBreaker)
		r.Get("/trades/history", h.GetTradesHistory)
		r.Get("/trades/closed", h.GetClosedTrades)
		r.Get("/stats/symbols", h.GetSymbolStats)
		r.Get("/stats/equity", h.GetEquityCurve)
		r.Post("/positions/{tradeID}/close-fictitious", h.ClosePositionFictitious)
		r.Put("/trades/{tradeID}", h.UpdateTrade)
	})
}

// Start starts the API server
func (s *BotServer) Start() error {
	log.Printf("Bot API server starting on %s", s.server.Addr)
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the server
func (s *BotServer) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}
