package cmd

import (
	_ "expvar" // Register expvar default http handler.
	"math/rand"
	"runtime"
	"time"

	"contrib.go.opencensus.io/exporter/jaeger"
	"github.com/getsentry/sentry-go"
	"github.com/go-redis/redis/v7"
	"github.com/urfave/cli/v2"
	"go.opencensus.io/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zapgrpc"
	"google.golang.org/grpc/grpclog"

	"github.com/Syncano/acme-proxy/app/version"
	"github.com/Syncano/pkg-go/v2/database"
	"github.com/Syncano/pkg-go/v2/log"
	"github.com/Syncano/pkg-go/v2/rediscache"
	"github.com/Syncano/pkg-go/v2/rediscli"
)

var (
	// App is the main structure of a cli application.
	App = cli.NewApp()

	dbOptions    = database.DefaultOptions
	redisOptions = redis.Options{}

	db             *database.DB
	jaegerExporter *jaeger.Exporter
	storRedis      *rediscli.Redis
	cache          *rediscache.Cache
	logger         *log.Logger
)

func init() {
	App.Name = "acme-proxy"
	App.Usage = "Application that enables running user provided unsecure code in a secure docker environment."
	App.Compiled = version.Buildtime
	App.Version = version.Current.String()
	App.Authors = []*cli.Author{
		{
			Name:  "Robert Kopaczewski",
			Email: "rk@23doors.com",
		},
	}
	App.Copyright = "Syncano"
	App.Flags = []cli.Flag{
		&cli.StringFlag{
			Name: "log-level", Usage: "logging level",
			EnvVars: []string{"LOG_LEVEL"}, Value: zapcore.InfoLevel.String(),
		},
		&cli.IntFlag{
			Name: "metrics-port", Aliases: []string{"mp"}, Usage: "port for metrics",
			EnvVars: []string{"METRICS_PORT"}, Value: 9080,
		},
		&cli.BoolFlag{
			Name: "debug", Usage: "enable debug mode",
			EnvVars: []string{"DEBUG"},
		},

		// Database options.
		&cli.StringFlag{
			Name: "db-name", Usage: "database name",
			EnvVars: []string{"DB_NAME", "PGDATABASE"}, Value: "acme-proxy", Destination: &dbOptions.Database,
		},
		&cli.StringFlag{
			Name: "db-user", Usage: "database user",
			EnvVars: []string{"DB_USER", "PGUSER"}, Value: "acme-proxy", Destination: &dbOptions.User,
		},
		&cli.StringFlag{
			Name: "db-pass", Usage: "database password",
			EnvVars: []string{"DB_PASS", "PGPASSWORD"}, Value: "acme-proxy", Destination: &dbOptions.Password,
		},
		&cli.StringFlag{
			Name: "db-host", Usage: "database host",
			EnvVars: []string{"DB_HOST", "PGHOST"}, Value: "postgresql", Destination: &dbOptions.Host,
		},
		&cli.StringFlag{
			Name: "db-port", Usage: "database port",
			EnvVars: []string{"DB_PORT", "PGPORT"}, Value: "5432", Destination: &dbOptions.Port,
		},

		// Tracing options.
		&cli.StringFlag{
			Name: "jaeger-collector-endpoint", Usage: "jaeger collector endpoint",
			EnvVars: []string{"JAEGER_COLLECTOR_ENDPOINT"}, Value: "http://jaeger:14268/api/traces",
		},
		&cli.Float64Flag{
			Name: "tracing-sampling", Usage: "tracing sampling probability value",
			EnvVars: []string{"TRACING_SAMPLING"}, Value: 0,
		},
		&cli.StringFlag{
			Name: "service-name", Aliases: []string{"n"}, Usage: "service name",
			EnvVars: []string{"SERVICE_NAME"}, Value: "acme-proxy",
		},

		// Redis options.
		&cli.StringFlag{
			Name: "redis-addr", Usage: "redis TCP address",
			EnvVars: []string{"REDIS_ADDR"}, Value: "redis:6379", Destination: &redisOptions.Addr,
		},
	}

	App.Before = func(c *cli.Context) error {
		// Initialize random seed.
		rand.Seed(time.Now().UnixNano())

		numCPUs := runtime.NumCPU()
		runtime.GOMAXPROCS(numCPUs + 1) // numCPUs hot threads + one for async tasks.

		// Initialize logging.
		if err := sentry.Init(sentry.ClientOptions{}); err != nil {
			return err
		}

		var err error
		if logger, err = log.New(sentry.CurrentHub().Client(),
			log.WithLogLevel(c.String("log_level")),
			log.WithDebug(c.Bool("debug")),
		); err != nil {
			return err
		}

		logg := logger.Logger()

		// Set grpc logger.
		var zapgrpcOpts []zapgrpc.Option
		if c.Bool("debug") {
			zapgrpcOpts = append(zapgrpcOpts, zapgrpc.WithDebug())
		}

		grpclog.SetLogger(zapgrpc.NewLogger(logg, zapgrpcOpts...)) // nolint: staticcheck

		// Initialize tracing.
		jaegerExporter, err = jaeger.NewExporter(jaeger.Options{
			CollectorEndpoint: c.String("jaeger-collector-endpoint"),
			Process: jaeger.Process{
				ServiceName: c.String("service-name"),
			},
			OnError: func(err error) {
				logg.With(zap.Error(err)).Warn("Jaeger tracing error")
			},
		})
		if err != nil {
			logg.With(zap.Error(err)).Fatal("Jaeger exporter misconfiguration")
		}

		trace.RegisterExporter(jaegerExporter)
		trace.ApplyConfig(trace.Config{
			DefaultSampler: trace.ProbabilitySampler(c.Float64("tracing-sampling")),
		})

		return nil
	}
	App.After = func(c *cli.Context) error {
		// Redis teardown.
		if storRedis != nil {
			storRedis.Shutdown() // nolint: errcheck
		}

		// Database teardown.
		if db != nil {
			db.Shutdown() // nolint: errcheck
		}

		// Sync loggers.
		if logger != nil {
			logger.Sync()
		}

		// Close tracing reporter.
		if jaegerExporter != nil {
			jaegerExporter.Flush()
		}

		// Flush remaining sentry events.
		sentry.Flush(5 * time.Second)

		return nil
	}
}
