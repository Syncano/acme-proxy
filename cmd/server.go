package cmd

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"contrib.go.opencensus.io/exporter/prometheus"
	"github.com/bsm/redislock"
	lego_log "github.com/go-acme/lego/v3/log"
	"github.com/urfave/cli/v2"
	"go.opencensus.io/plugin/ocgrpc"
	"go.opencensus.io/plugin/ochttp"
	"go.opencensus.io/stats/view"
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/Syncano/acme-proxy/app/query"
	"github.com/Syncano/acme-proxy/app/server"
	"github.com/Syncano/acme-proxy/app/version"
	"github.com/Syncano/acme-proxy/pkg/acme"
	"github.com/Syncano/acme-proxy/pkg/cert"
	"github.com/Syncano/pkg-go/v2/database"
	"github.com/Syncano/pkg-go/v2/jobs"
	"github.com/Syncano/pkg-go/v2/rediscache"
	"github.com/Syncano/pkg-go/v2/rediscli"
	pb "github.com/Syncano/syncanoapis/gen/go/syncano/hosting/acme/v1"
)

var serverCmd = &cli.Command{
	Name:  "server",
	Usage: "Starts acme proxy server.",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name: "http-port", Usage: "port for http",
			EnvVars: []string{"HTTP_PORT"}, Value: 8000,
		},
		&cli.IntFlag{
			Name: "https-port", Usage: "port for https",
			EnvVars: []string{"HTTPS_PORT"}, Value: 8010,
		},
		&cli.IntFlag{
			Name: "grpc-port", Usage: "port for grpc",
			EnvVars: []string{"GRPC_PORT"}, Value: 9000,
		},

		// Acme settings.
		&cli.StringFlag{
			Name: "acme-email", Usage: "acme email",
			EnvVars: []string{"ACME_EMAIL"}, Value: "test@example.com",
		},
		&cli.StringFlag{
			Name: "acme-ca-dir", Usage: "acme CA directory",
			EnvVars: []string{"ACME_CA_DIR"}, Value: "https://acme-staging-v02.api.letsencrypt.org/directory",
		},

		// Cert storage settings.
		&cli.BoolFlag{
			Name: "local-certs-enabled", Usage: "load local certs from certs path on start",
			EnvVars: []string{"LOCAL_CERTS_ENABLED"}, Value: true,
		},
		&cli.DurationFlag{
			Name: "local-certs-refresh-period", Usage: "local certs refresh period",
			EnvVars: []string{"LOCAL_CERTS_REFRESH_PERIOD"}, Value: 12 * time.Hour,
		},
		&cli.StringFlag{
			Name: "local-certs-path", Usage: "certs path to load",
			EnvVars: []string{"LOCAL_CERTS_PATH"}, Value: "./certs",
		},
		&cli.StringFlag{
			Name: "cert-default", Usage: "default cert server name to use as a fallback",
			EnvVars: []string{"CERT_DEFAULT"},
		},

		// Proxy settings.
		&cli.StringFlag{
			Name: "routes-file", Usage: "routes file path",
			EnvVars: []string{"ROUTES_FILE"}, Value: "./routes.yaml",
		},

		// Cache settings.
		&cli.IntFlag{
			Name: "cache-version", Usage: "cache version",
			EnvVars: []string{"CACHE_VERSION"}, Value: 1,
		},
		&cli.DurationFlag{
			Name: "cache-timeout", Usage: "cache timeout",
			EnvVars: []string{"CACHE_TIMEOUT"}, Value: 12 * time.Hour,
		},
		&cli.DurationFlag{
			Name: "local-cache-timeout", Usage: "local cache version",
			EnvVars: []string{"LOCAL_CACHE_TIMEOUT"}, Value: 1 * time.Hour,
		},
	},
	Action: func(c *cli.Context) error {
		logg := logger.Logger()

		// Setup prometheus handler.
		exporter, err := prometheus.NewExporter(prometheus.Options{})
		if err != nil {
			logg.With(zap.Error(err)).Fatal("Prometheus exporter misconfiguration")
		}

		var views []*view.View
		views = append(views, ochttp.DefaultClientViews...)
		views = append(views, ochttp.DefaultServerViews...)
		views = append(views, ocgrpc.DefaultClientViews...)
		views = append(views, ocgrpc.DefaultServerViews...)

		if err := view.Register(views...); err != nil {
			logg.With(zap.Error(err)).Fatal("Opencensus views registration failed")
		}

		// Serve prometheus metrics.
		http.Handle("/metrics", exporter)

		// Setup health check.
		http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(200)
		})

		// Initialize database client.
		db = database.NewDB(&dbOptions, nil, logger, c.Bool("debug"))

		// Initialize redis client.
		storRedis = rediscli.NewRedis(&redisOptions)
		cache = rediscache.New(storRedis.Client(), db,
			rediscache.WithServiceKey("acme"),
			rediscache.WithTimeout(c.Duration("local-cache-timeout"), c.Duration("cache-timeout")),
			rediscache.WithVersion(c.Int("cache-version")),
		)

		logg.With(
			zap.String("version", App.Version),
			zap.String("gitsha", version.GitSHA),
			zap.Time("buildtime", App.Compiled),
		).Info("Server starting")

		// Initialize query factory.
		qf := query.NewFactory(db, cache)

		// Initialize jobRunner
		jobRunner := jobs.New(logg, storRedis.Client(), jobs.WithServiceKey("acme"))

		// Initialize acme client.
		lego_log.Logger, _ = zap.NewStdLogAt(logger.Logger(), zap.DebugLevel)

		locker := redislock.New(storRedis.Client())
		acmeClient := acme.NewClient(
			c.String("acme-ca-dir"),
			cache, locker, logger)

		// Initialize cert storage.
		stor := cert.NewStorage(
			c.String("cert-default"),
			cache)
		if c.Bool("local-certs-enabled") {
			path := c.String("local-certs-path")

			if err := stor.LoadDir(path); err != nil {
				return err
			}

			go func() {
				timer := time.NewTimer(c.Duration("local-certs-refresh-period"))

				for {
					<-timer.C
					if err := stor.LoadDir(path); err != nil {
						logg.With(zap.Error(err)).Error("Load certificates during refresh")
					}
				}
			}()
		}

		// Create new http server.
		srv, err := server.New(
			jobRunner,
			acmeClient,
			qf,
			logger,
			stor,
			c.String("acme-email"),
			c.String("routes-file"),
			c.Bool("debug"))
		if err != nil {
			return err
		}

		webServer := srv.HTTPServer()

		httpLis, err := net.Listen("tcp", fmt.Sprintf(":%d", c.Int("http-port")))
		if err != nil {
			return err
		}

		httpsLis, err := net.Listen("tcp", fmt.Sprintf(":%d", c.Int("https-port")))
		if err != nil {
			return err
		}

		go func() {
			if err := webServer.Serve(httpLis); err != nil && err != http.ErrServerClosed {
				logg.With(zap.Error(err)).Fatal("HTTP serve error")
			}
		}()
		go func() {
			if err := webServer.ServeTLS(httpsLis, "", ""); err != nil && err != http.ErrServerClosed {
				logg.With(zap.Error(err)).Fatal("HTTPS serve error")
			}
		}()

		logg.With(
			zap.Int("http-port", c.Int("http-port")),
			zap.Int("https-port", c.Int("https-port"))).Info("Serving web")

		// Create new grpc server.
		grpcLis, err := net.Listen("tcp", fmt.Sprintf(":%d", c.Int("grpc-port")))
		if err != nil {
			return err
		}
		grpcServer := grpc.NewServer(
			grpc.StatsHandler(&ocgrpc.ServerHandler{}),
		)

		// Register all servers.
		pb.RegisterAcmeProxyServer(grpcServer, srv)

		// Serve a new gRPC service.
		go func() {
			if err = grpcServer.Serve(grpcLis); err != nil && err != grpc.ErrServerStopped {
				logg.With(zap.Error(err)).Fatal("gRPC serve error")
			}
		}()
		logg.With(zap.Int("port", c.Int("grpc-port"))).Info("Serving gRPC")

		// Serve metrics.
		logg.With(zap.Int("metrics-port", c.Int("metrics-port"))).Info("Serving http for metrics")
		metricsServer := http.Server{Addr: fmt.Sprintf(":%d", c.Int("metrics-port"))}

		go func() {
			if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logg.With(zap.Error(err)).Fatal("Serve error")
			}
		}()

		// Handle SIGINT and SIGTERM.
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
		<-ch

		// Graceful shutdown.
		logg.Info("Shutting down")
		jobRunner.GracefulStop()
		metricsServer.Shutdown(context.Background()) // nolint: errcheck
		webServer.Shutdown(context.Background())     // nolint: errcheck
		grpcServer.GracefulStop()
		return nil
	},
}

func init() {
	App.Commands = append(App.Commands, serverCmd)
}
