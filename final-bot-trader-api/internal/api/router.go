package api

import (
	"net/http"
	"time"

	"final-bot-trader-api/internal/api/handlers"
	"final-bot-trader-api/internal/api/middleware"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// Server holds all dependencies for the API server
type Server struct {
	Router  *chi.Mux
	Handler *handlers.Handler
}

// ServerConfig holds server configuration
type ServerConfig struct {
	// Exchange client for trading operations
	// ExchangeClient exchange.Client

	// Enable debug logging
	Debug bool

	// CORS configuration
	AllowedOrigins []string
}

// DefaultServerConfig returns default server configuration
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		Debug:          false,
		AllowedOrigins: []string{"*"},
	}
}

// NewServer creates a new API server with all routes configured
func NewServer(config ServerConfig) *Server {
	r := chi.NewRouter()

	// Global middleware
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(60 * time.Second))

	// CORS middleware
	r.Use(middleware.CORS(config.AllowedOrigins))

	// Create handler with dependencies
	h := handlers.NewHandler()

	// Setup routes
	setupRoutes(r, h)

	return &Server{
		Router:  r,
		Handler: h,
	}
}

func setupRoutes(r *chi.Mux, h *handlers.Handler) {
	// Health check (no prefix)
	r.Get("/health", h.HealthCheck)
	r.Get("/", h.Welcome)

	// API v1 routes
	r.Route("/api/v1", func(r chi.Router) {
		// Market data
		r.Route("/market", func(r chi.Router) {
			r.Get("/price/{symbol}", h.GetPrice)
			r.Get("/klines/{symbol}", h.GetKlines)
		})

		// Account
		r.Route("/account", func(r chi.Router) {
			r.Get("/", h.GetAccount)
			r.Get("/balance", h.GetBalance)
			r.Get("/positions", h.GetPositions)
		})

		// Trading
		r.Route("/trading", func(r chi.Router) {
			r.Get("/orders", h.GetOrders)
			r.Post("/orders", h.PlaceOrder)
			r.Delete("/orders/{orderID}", h.CancelOrder)
			r.Post("/positions/{symbol}/close", h.ClosePosition)
		})

		// Backtesting
		r.Route("/backtest", func(r chi.Router) {
			r.Post("/run", h.RunBacktest)
			r.Get("/strategies", h.ListStrategies)
		})

		// Strategies
		r.Route("/strategies", func(r chi.Router) {
			r.Get("/", h.ListStrategies)
			r.Get("/{name}", h.GetStrategy)
			r.Post("/{name}/analyze", h.AnalyzeWithStrategy)
		})
	})
}

// ServeHTTP implements http.Handler
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.Router.ServeHTTP(w, r)
}
