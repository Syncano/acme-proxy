package settings

import (
	"time"

	"github.com/caarlos0/env/v6"

	"github.com/Syncano/pkg-go/v2/util"
)

type server struct {
	UserFetchTimeout    time.Duration `env:"USER_FETCH_TIMEOUT"`
	CertFetchTimeout    time.Duration `env:"CERT_FETCH_TIMEOUT"`
	DomainVerifyTimeout time.Duration `env:"DOMAIN_VERIFY_TIMEOUT"`

	ServerReadTimeout  time.Duration `env:"SERVER_READ_TIMEOUT"`
	ServerWriteTimeout time.Duration `env:"SERVER_WRITE_TIMEOUT"`
	ServerIdleTimeout  time.Duration `env:"SERVER_IDLE_TIMEOUT"`

	AutoRefreshBatch        int           `env:"AUTO_REFRESH_BATCH"`
	RefreshFailureThreshold int           `env:"REFRESH_FAILURE_THRESHOLD"`
	CertRefreshTimeout      time.Duration `env:"CERT_REFRESH_TIMEOUT"`
	CertRefreshPeriod       time.Duration `env:"CERT_REFRESH_PERIOD"`

	CertListLimit int `env:"CERT_LIST_LIMIT"`
}

var Server = &server{
	UserFetchTimeout:    30 * time.Second,
	CertFetchTimeout:    10 * time.Second,
	DomainVerifyTimeout: 5 * time.Second,
	ServerReadTimeout:   1 * time.Minute,
	ServerWriteTimeout:  6 * time.Minute,
	ServerIdleTimeout:   2 * time.Minute,

	AutoRefreshBatch:        10,
	RefreshFailureThreshold: 3,
	CertRefreshTimeout:      30 * time.Second,
	CertRefreshPeriod:       6 * time.Hour,

	CertListLimit: 100,
}

func init() {
	util.Must(env.Parse(Server))
}
