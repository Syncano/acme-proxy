package cert

import (
	"crypto/tls"

	"github.com/Syncano/pkg-go/v2/rediscache"
)

type Storage struct {
	certDefault string

	local map[string]*tls.Certificate
	rc    *rediscache.Cache
}

func NewStorage(certDefault string, rc *rediscache.Cache) *Storage {
	return &Storage{
		certDefault: certDefault,
		local:       make(map[string]*tls.Certificate),
		rc:          rc,
	}
}

func (cs *Storage) LoadDir(dir string) error {
	certs, err := LoadCertificatesFromPath(dir)
	if err != nil {
		return err
	}

	for k, v := range certs {
		cs.local[k] = v
	}

	return nil
}
