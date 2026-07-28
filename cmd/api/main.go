package main

import (
	"context"
	"errors"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	_ "github.com/keelwave/keelwave/docs"
	"github.com/keelwave/keelwave/internal/alerting"
	"github.com/keelwave/keelwave/internal/alerting/channels"
	"github.com/keelwave/keelwave/internal/batch"
	"github.com/keelwave/keelwave/internal/db"
	"github.com/keelwave/keelwave/internal/env"
	"github.com/keelwave/keelwave/internal/mailer"
	"github.com/keelwave/keelwave/internal/store"
)

var version = "0.0.1"

//	@title			Keelwave API
//	@description	Unified observability platform for AI agents, APIs, and infrastructure.
//	@version		0.0.1

// @BasePath					/v1
//
// @securityDefinitions.apikey	ApiKeyAuth
// @in							header
// @name						Authorization
// @description				Format: "Bearer kw_<token>"
func main() {
	cfg := config{
		addr:        env.GetString("ADDR", ":8080"),
		env:         env.GetString("ENV", "development"),
		corsOrigins: env.GetStringSlice("CORS_ALLOWED_ORIGINS", []string{"http://localhost:3000"}),
		db: dbConfig{
			addr:     env.GetString("DB_ADDR", "postgres://keelwave:keelwave@localhost:5432/keelwave?sslmode=disable"),
			maxConns: int32(env.GetInt("DB_MAX_CONNS", 30)),
		},
		rateLimit: rateLimitConfig{
			ingestIPPerMinute:  env.GetInt("RATE_LIMIT_INGEST_IP_PER_MINUTE", 100),
			ingestKeyPerMinute: env.GetInt("RATE_LIMIT_INGEST_KEY_PER_MINUTE", 1000),
			ingestWindow:       parseDurationOr("RATE_LIMIT_INGEST_WINDOW", time.Minute),
		},
		batch: batchConfig{
			flushInterval: parseDurationOr("BATCH_FLUSH_INTERVAL", 500*time.Millisecond),
			maxRows:       env.GetInt("BATCH_MAX_ROWS", 500),
			queueDepth:    env.GetInt("BATCH_QUEUE_DEPTH", 10_000),
		},
		auth: authConfig{
			sessionTTL:   parseDurationOr("SESSION_TTL", 7*24*time.Hour),
			cookieName:   env.GetString("SESSION_COOKIE_NAME", "keelwave_session"),
			cookieSecure: env.GetString("ENV", "development") == "production",
			publicURL:    env.GetString("PUBLIC_URL", "http://localhost:8080"),
			dashboardURL: env.GetString("DASHBOARD_URL", "http://localhost:3000"),
			google: oauthProvider{
				clientID:     env.GetString("GOOGLE_CLIENT_ID", ""),
				clientSecret: env.GetString("GOOGLE_CLIENT_SECRET", ""),
			},
			github: oauthProvider{
				clientID:     env.GetString("GITHUB_CLIENT_ID", ""),
				clientSecret: env.GetString("GITHUB_CLIENT_SECRET", ""),
			},
		},
		mail: mailConfig{
			apiKey:    env.GetString("RESEND_API_KEY", ""),
			fromEmail: env.GetString("MAIL_FROM", "example@example.com"),
		},
		shutdownTimeout: parseDurationOr("SHUTDOWN_TIMEOUT", 10*time.Second),
	}

	logger := zap.Must(zap.NewProduction()).Sugar()
	defer logger.Sync()

	for _, origin := range cfg.corsOrigins {
		if origin == "*" {
			logger.Fatal("CORS_ALLOWED_ORIGINS must not contain \"*\" when AllowCredentials is true; specify explicit origins")
		}
	}

	rootCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := db.New(rootCtx, cfg.db.addr, cfg.db.maxConns)
	if err != nil {
		logger.Fatalw("db connect failed", "err", err)
	}
	logger.Info("db connected")

	batchers := batch.NewBatchers(pool, batch.Config{
		FlushInterval: cfg.batch.flushInterval,
		MaxRows:       cfg.batch.maxRows,
		QueueDepth:    cfg.batch.queueDepth,
	}, logger)

	batchers.Start(rootCtx)

	app := &application{
		config:   cfg,
		pool:     pool,
		store:    store.NewStorage(pool),
		batchers: batchers,
		mailer:   mailer.NewResendClient(cfg.mail.apiKey, cfg.mail.fromEmail),
		logger:   logger,
	}

	reg := channels.Registry{"email": channels.NewEmail(app.mailer)}
	evaluator := alerting.NewEvaluator(pool, app.store, logger)
	worker := alerting.NewWorker(app.store, reg, logger)
	scheduler := alerting.NewScheduler(evaluator, app.store, logger,
		parseDurationOr("ALERT_EVAL_INTERVAL", 30*time.Second))
	app.evaluator = evaluator
	app.worker = worker
	app.scheduler = scheduler
	worker.Start(rootCtx)
	scheduler.Start(rootCtx)

	mux := app.mount()
	srvErr := make(chan error, 1)
	go func() {
		if err := app.run(mux); err != nil && !errors.Is(err, http.ErrServerClosed) {
			srvErr <- err
			return
		}
		srvErr <- nil
	}()

	select {
	case err := <-srvErr:
		if err != nil {
			logger.Errorw("server exited with error", "err", err)
		}
	case <-rootCtx.Done():
		logger.Infow("shutdown initiated")
	}

	shutCtx, cancel := context.WithTimeout(context.Background(), cfg.shutdownTimeout)
	defer cancel()

	if app.srv != nil {
		if err := app.srv.Shutdown(shutCtx); err != nil {
			logger.Warnw("server shutdown error", "err", err)
		}
	}
	if err := batchers.Stop(shutCtx); err != nil {
		logger.Warnw("batch drain incomplete", "err", err)
	}
	if err := scheduler.Stop(shutCtx); err != nil {
		logger.Warnw("scheduler stop", "err", err)
	}
	if err := worker.Stop(shutCtx); err != nil {
		logger.Warnw("worker stop", "err", err)
	}
	pool.Close()
	logger.Info("shutdown complete")
}

func parseDurationOr(key string, def time.Duration) time.Duration {
	s := env.GetString(key, "")
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return def
	}
	return d
}
