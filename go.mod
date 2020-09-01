module github.com/Syncano/acme-proxy

go 1.15

require (
	contrib.go.opencensus.io/exporter/jaeger v0.2.1
	contrib.go.opencensus.io/exporter/prometheus v0.2.0
	github.com/Syncano/pkg-go/v2 v2.7.4
	github.com/Syncano/syncanoapis/gen v1.2.0
	github.com/blang/semver v3.5.1+incompatible
	github.com/bsm/redislock v0.5.0
	github.com/caarlos0/env/v6 v6.3.0
	github.com/cespare/reflex v0.3.0
	github.com/cpuguy83/go-md2man/v2 v2.0.0 // indirect
	github.com/getsentry/sentry-go v0.7.0
	github.com/go-acme/lego/v3 v3.8.0
	github.com/go-pg/pg/v9 v9.1.7
	github.com/go-redis/redis/v7 v7.4.0
	github.com/golang/protobuf v1.4.2
	github.com/json-iterator/go v1.1.10
	github.com/labstack/echo/v4 v4.1.17
	github.com/lib/pq v1.8.0
	github.com/pressly/goose v2.6.0+incompatible
	github.com/prometheus/client_golang v1.7.1 // indirect
	github.com/shopspring/decimal v1.2.0 // indirect
	github.com/smartystreets/goconvey v1.6.4
	github.com/urfave/cli/v2 v2.2.0
	github.com/vektra/mockery v1.1.2
	go.opencensus.io v0.22.4
	go.uber.org/zap v1.15.0
	golang.org/x/tools v0.0.0-20200828161849-5deb26317202
	google.golang.org/grpc v1.31.1
	google.golang.org/protobuf v1.25.0
	gopkg.in/square/go-jose.v2 v2.5.1
	gopkg.in/yaml.v2 v2.3.0
)
