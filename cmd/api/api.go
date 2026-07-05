package main

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/keelwave/keelwave/internal/alerting"
	"github.com/keelwave/keelwave/internal/batch"
	"github.com/keelwave/keelwave/internal/mailer"
	"github.com/keelwave/keelwave/internal/store"
)

type application struct {
	config    config
	pool      *pgxpool.Pool
	store     store.Storage
	batchers  *batch.Batchers
	mailer    mailer.Client
	logger    *zap.SugaredLogger
	evaluator *alerting.Evaluator
	scheduler *alerting.Scheduler
	worker    *alerting.Worker
	srv       *http.Server
}

type config struct {
	addr            string
	env             string
	corsOrigins     []string
	db              dbConfig
	rateLimit       rateLimitConfig
	batch           batchConfig
	auth            authConfig
	mail            mailConfig
	shutdownTimeout time.Duration
}

type mailConfig struct {
	apiKey    string
	fromEmail string
}

type authConfig struct {
	sessionTTL   time.Duration
	cookieName   string
	cookieSecure bool
	publicURL    string
	dashboardURL string
	google       oauthProvider
	github       oauthProvider
}

type oauthProvider struct {
	clientID     string
	clientSecret string
}

func (p oauthProvider) configured() bool {
	return p.clientID != "" && p.clientSecret != ""
}

type dbConfig struct {
	addr     string
	maxConns int32
}

type rateLimitConfig struct {
	ingestIPPerMinute  int
	ingestKeyPerMinute int
	ingestWindow       time.Duration
}

type batchConfig struct {
	flushInterval time.Duration
	maxRows       int
	queueDepth    int
}

func (app *application) mount() *chi.Mux {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   app.config.corsOrigins,
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-API-Key", "X-Project-ID", "X-Org-ID"},
		AllowCredentials: true,
		MaxAge:           300,
	}))
	r.Use(middleware.Timeout(60 * time.Second))

	r.Route("/v1", func(r chi.Router) {
		r.Get("/health", app.healthHandler)
		r.Get("/openapi.json", app.openAPIHandler)
		r.Get("/docs", app.docsHandler)

		r.Route("/auth", func(r chi.Router) {
			r.Post("/register", app.registerHandler)
			r.Post("/login", app.loginHandler)
			r.Get("/oauth/{provider}/start", app.oauthStartHandler)
			r.Get("/oauth/{provider}/callback", app.oauthCallbackHandler)
			r.Get("/invites/{token}", app.getInviteHandler)
			r.Get("/verify-email/{token}", app.verifyEmailHandler)
			r.Group(func(r chi.Router) {
				r.Use(app.userAuthMiddleware)
				r.Post("/logout", app.logoutHandler)
				r.Get("/me", app.meHandler)
				r.Put("/invites/{token}/accept", app.acceptInviteHandler)
			})
		})

		r.Route("/ingest", func(r chi.Router) {
			r.Use(app.ingestIPRateLimit())
			r.Use(app.apiKeyAuthMiddleware)
			r.Use(app.ingestKeyRateLimit())
			r.Post("/ai", app.ingestAIHandler)
			r.Post("/events", app.ingestEventHandler)
			r.Post("/metrics", app.ingestMetricHandler)

			r.Route("/agent", func(r chi.Router) {
				r.Post("/runs", app.ingestAgentRunStartHandler)
				r.Post("/runs/{runID}/finish", app.ingestAgentRunFinishHandler)
				r.Post("/steps", app.ingestAgentStepHandler)
			})
		})

		r.Route("/projects/{projectID}", func(r chi.Router) {
			r.Route("/agent", func(r chi.Router) {
				r.Use(app.queryAuthMiddleware)
				r.Use(app.requireVerifiedMiddleware)
				r.Get("/health", app.runHealthHandler)
				r.Get("/tools/stats", app.toolStatsHandler)
				r.Get("/summary", app.summaryHandler)
				r.Get("/steps/distribution", app.stepDistributionHandler)

				r.Route("/runs", func(r chi.Router) {
					r.Get("/", app.listAgentRunsHandler)
					r.Get("/timeseries", app.runsTimeseriesHandler)
					r.Get("/terminations", app.terminationsHandler)
					r.Get("/{runID}", app.getAgentRunHandler)
					r.Get("/{runID}/steps", app.listAgentStepsHandler)
					r.Get("/{runID}/loops", app.listAgentLoopsHandler)
				})
			})
		})

		r.Route("/admin", func(r chi.Router) {
			r.Use(app.userAuthMiddleware)
			r.Use(app.requireVerifiedMiddleware)
			r.Post("/orgs", app.createOrgHandler)

			r.Route("/orgs/{orgID}", func(r chi.Router) {
				r.With(app.requireOrgRoleMiddleware(RoleLevelAdmin)).Patch("/", app.updateOrgHandler)
				r.With(app.requireOrgRoleMiddleware(RoleLevelOwner)).Delete("/", app.deleteOrgHandler)

				r.Route("/members", func(r chi.Router) {
					r.With(app.requireOrgRoleMiddleware(RoleLevelMember)).Get("/", app.listMembersHandler)
					r.With(app.requireOrgRoleMiddleware(RoleLevelOwner)).Patch("/{userID}/role", app.updateMemberRoleHandler)
					r.With(app.requireOrgRoleMiddleware(RoleLevelAdmin)).Delete("/{userID}", app.removeMemberHandler)
				})

				r.Route("/invites", func(r chi.Router) {
					r.Use(app.requireOrgRoleMiddleware(RoleLevelAdmin))
					r.Post("/", app.createInviteHandler)
					r.Get("/", app.listInvitesHandler)
					r.Delete("/{inviteID}", app.deleteInviteHandler)
				})

				r.Route("/projects", func(r chi.Router) {
					r.With(app.requireOrgRoleMiddleware(RoleLevelMember)).Get("/", app.listProjectsHandler)
					r.With(app.requireOrgRoleMiddleware(RoleLevelAdmin)).Post("/", app.createProjectHandler)

					r.Route("/{projectID}", func(r chi.Router) {
						r.With(app.requireOrgRoleMiddleware(RoleLevelMember)).Get("/", app.getProjectHandler)
						r.With(app.requireOrgRoleMiddleware(RoleLevelAdmin)).Delete("/", app.deleteProjectHandler)

						r.Route("/keys", func(r chi.Router) {
							r.With(app.requireOrgRoleMiddleware(RoleLevelMember)).Get("/", app.listKeysHandler)
							r.With(app.requireOrgRoleMiddleware(RoleLevelAdmin)).Post("/", app.createKeyHandler)
							r.With(app.requireOrgRoleMiddleware(RoleLevelAdmin)).Delete("/{keyID}", app.deleteKeyHandler)
						})

						r.Route("/alert-rules", func(r chi.Router) {
							r.With(app.requireOrgRoleMiddleware(RoleLevelMember)).Get("/", app.listAlertRulesHandler)
							r.With(app.requireOrgRoleMiddleware(RoleLevelAdmin)).Post("/", app.createAlertRuleHandler)
							r.With(app.requireOrgRoleMiddleware(RoleLevelAdmin)).Patch("/{ruleID}", app.updateAlertRuleHandler)
							r.With(app.requireOrgRoleMiddleware(RoleLevelAdmin)).Delete("/{ruleID}", app.deleteAlertRuleHandler)
						})
					})
				})
			})
		})
	})

	return r
}

func (app *application) run(mux http.Handler) error {
	app.srv = &http.Server{
		Addr:         app.config.addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  time.Minute,
	}

	app.logger.Infow("server started", "addr", app.config.addr, "env", app.config.env, "version", version)
	return app.srv.ListenAndServe()
}
