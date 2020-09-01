package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/http"
	_ "net/http/pprof" // nolint:gosec // using pprof when debug is true
	"strings"

	sentryecho "github.com/getsentry/sentry-go/echo"
	"github.com/go-pg/pg/v9"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
	"gopkg.in/yaml.v2"

	"github.com/Syncano/acme-proxy/app/models"
	"github.com/Syncano/acme-proxy/app/query"
	"github.com/Syncano/acme-proxy/app/settings"
	"github.com/Syncano/acme-proxy/pkg/acme"
	"github.com/Syncano/acme-proxy/pkg/cert"
	echo_middleware "github.com/Syncano/pkg-go/v2/echo_middleware"
	"github.com/Syncano/pkg-go/v2/jobs"
	"github.com/Syncano/pkg-go/v2/log"
)

// Server defines a Web server.
type Server struct {
	jobRunner *jobs.Runner
	acme      *acme.Client
	qf        *query.Factory
	log       *log.Logger
	cert      *cert.Storage
	routes    []*Route
	user      *models.User
	debug     bool
}

const certRefreshName = "server.CertAutoRefresh"

// New initializes new server.
func New(jobRunner *jobs.Runner, acmeClient *acme.Client, qf *query.Factory, logger *log.Logger, stor *cert.Storage,
	email, routeFilePath string, debug bool) (*Server, error) {
	// Load routes.
	routesFile, err := ioutil.ReadFile(routeFilePath)
	if err != nil {
		return nil, err
	}

	var routes []*Route

	err = yaml.Unmarshal(routesFile, &routes)
	if err != nil {
		return nil, err
	}

	s := &Server{
		jobRunner: jobRunner,
		acme:      acmeClient,
		qf:        qf,
		log:       logger,
		cert:      stor,
		routes:    routes,
		debug:     debug,
	}

	// Load user.
	ctx, cancel := context.WithTimeout(context.Background(), settings.Server.UserFetchTimeout)
	defer cancel()

	mgr := qf.NewUserManager(ctx)
	user := &models.User{}

	err = mgr.RunInTransaction(func(tx *pg.Tx) error {
		err := mgr.LockTable()
		if err != nil {
			return err
		}

		err = mgr.First(user)
		if err != nil && err != pg.ErrNoRows {
			return err
		}

		// If user already exists, use it.
		if err == nil {
			acmeUser, err := user.AcmeUser()
			if err != nil {
				return err
			}

			_, err = acmeClient.InitializeUser(acmeUser)

			return err
		}

		// Register new user.
		acmeUser, err := acmeClient.InitializeUser(&acme.User{Email: email})
		if err != nil {
			return err
		}

		user, err = models.NewUserFromAcmeUser(acmeUser)
		if err != nil {
			return err
		}

		err = mgr.Insert(user)
		if err != nil {
			return err
		}

		return nil
	})

	s.user = user

	s.jobRunner.Run(&jobs.PeriodicJob{
		Name:    certRefreshName,
		Func:    s.certAutoRefresh,
		Timeout: settings.Server.CertRefreshTimeout,
		Period:  settings.Server.CertRefreshPeriod,
	})

	return s, err
}

func (s *Server) HTTPServer() *http.Server {
	stdlog, _ := zap.NewStdLogAt(s.log.Logger(), zap.WarnLevel)

	return &http.Server{
		ReadTimeout:  settings.Server.ServerReadTimeout,
		WriteTimeout: settings.Server.ServerWriteTimeout,
		IdleTimeout:  settings.Server.ServerIdleTimeout,
		ErrorLog:     stdlog,
		TLSConfig: &tls.Config{ //nolint:gosec
			// Causes servers to use Go's default ciphersuite preferences,
			// which are tuned to avoid attacks. Does nothing on clients.
			PreferServerCipherSuites: true,
			// Only use curves which have assembly implementations
			CurvePreferences: []tls.CurveID{
				tls.CurveP256,
				tls.X25519, // Go 1.8 only
			},
			MinVersion:     tls.VersionTLS10,
			GetCertificate: s.getCertificate,
		},
		Handler: s.setupRouter(),
	}
}

func (s *Server) getCertificate(hi *tls.ClientHelloInfo) (*tls.Certificate, error) {
	domain := strings.TrimSpace(strings.ToLower(hi.ServerName))

	ctx, cancel := context.WithTimeout(context.Background(), settings.Server.CertFetchTimeout)
	defer cancel()

	return s.cert.Match(domain, func(domain string) (*tls.Certificate, error) {
		o := &models.Certificate{
			Domain: domain,
		}

		err := s.qf.NewCertManager(ctx).OneByDomain(o)
		if err != nil {
			if err == pg.ErrNoRows {
				return nil, nil
			}

			return nil, err
		}

		return o.TLSCertificate()
	})
}

func (s *Server) setupRouter() http.Handler {
	e := echo.New()
	// Bottom up middlewares
	e.Use(
		echo_middleware.RequestID(),
		middleware.CORSWithConfig(middleware.CORSConfig{MaxAge: 86400}),
		echo_middleware.OpenCensus(),
		sentryecho.New(sentryecho.Options{
			Repanic: true,
		}),
		echo_middleware.Logger(s.log),
		echo_middleware.Recovery(s.log),
	)

	// Register profiling if debug is on.
	// go tool pprof http://.../debug/pprof/profile
	// go tool pprof http://.../debug/pprof/heap
	if s.debug {
		e.GET("/debug/pprof/*", echo.WrapHandler(http.DefaultServeMux))
	}

	e.GET("/.well-known/acme-challenge/:acme_key", func(c echo.Context) error {
		acmeKey := c.Param("acme_key")
		thumb, err := s.acme.Thumbprint()
		if err != nil {
			return err
		}

		return c.String(http.StatusOK, fmt.Sprintf("%s.%s", acmeKey, thumb))
	})

	e.GET("/.well-known/echo/:acme_key", func(c echo.Context) error {
		return c.String(http.StatusOK, c.Param("acme_key"))
	})

	// Create proxy balancer targets.
	for _, r := range s.routes {
		var (
			balancer middleware.ProxyBalancer
			targets  []*middleware.ProxyTarget
		)

		for _, t := range r.Targets {
			targets = append(targets, &middleware.ProxyTarget{
				URL: t.URL,
			})
		}

		balancer = middleware.NewRoundRobinBalancer(targets)

		e.Group(r.Path, func(next echo.HandlerFunc) echo.HandlerFunc {
			return func(c echo.Context) error {
				h := c.Request().Header

				for k, v := range r.Headers { // nolint:scopelint
					h.Set(k, v)
				}

				return next(c)
			}
		}, middleware.ProxyWithConfig(middleware.ProxyConfig{
			Balancer: balancer,
		}))
	}

	return e
}
