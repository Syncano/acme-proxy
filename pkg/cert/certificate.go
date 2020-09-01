package cert

import (
	"crypto/tls"
	"errors"
	"strings"

	"github.com/Syncano/pkg-go/v2/rediscache"
)

type Certificate struct {
	Certificate [][]byte
	PrivateKey  []byte
	KeyType     KeyType
}

const matchKey = "cert.Storage.Match"

func NewCertificate(cert *tls.Certificate) (*Certificate, error) {
	priv, typ, err := MarshalPrivateKey(cert.PrivateKey)
	if err != nil {
		return nil, err
	}

	return &Certificate{
		Certificate: cert.Certificate,
		PrivateKey:  priv,
		KeyType:     typ,
	}, nil
}

func (c *Certificate) TLSCertificate() (*tls.Certificate, error) {
	priv, err := ParsePrivateKeyByType(c.PrivateKey, c.KeyType)

	return &tls.Certificate{
		Certificate: c.Certificate,
		PrivateKey:  priv,
	}, err
}

func (cs *Storage) matchSingleDomain(domain string, resolve func(domain string) (*tls.Certificate, error)) (*Certificate, error) {
	labels := strings.SplitN(domain, ".", 3)
	labels[0] = "*"
	wildcardDomain := strings.Join(labels, ".")

	for sans, crt := range cs.local {
		for _, certDomain := range strings.Split(sans, ",") {
			if MatchDomain(domain, certDomain) {
				return NewCertificate(crt)
			}
		}
	}

	crt, err := resolve(domain)
	if err != nil {
		return nil, err
	}

	if crt != nil {
		return NewCertificate(crt)
	}

	// Try wildcard if domain has more than 2 parts.
	if len(labels) > 2 {
		for sans, crt := range cs.local {
			for _, certDomain := range strings.Split(sans, ",") {
				if MatchDomain(wildcardDomain, certDomain) {
					return NewCertificate(crt)
				}
			}
		}

		crt, err = resolve(wildcardDomain)
		if err != nil {
			return nil, err
		}

		if crt != nil {
			return NewCertificate(crt)
		}
	}

	return nil, nil
}

func (cs *Storage) matchDomain(domain string, resolve func(domain string) (*tls.Certificate, error)) (*tls.Certificate, error) {
	var crt Certificate

	labels := strings.SplitN(domain, ".", 2)
	toplevelDomain := "." + strings.Join(labels[1:], ".")

	err := cs.rc.SimpleFuncCache(matchKey, domain, toplevelDomain, &crt, func() (interface{}, error) {
		crt, err := cs.matchSingleDomain(domain, resolve)
		if err != nil {
			return nil, err
		}

		if crt != nil {
			return crt, nil
		}

		if cs.certDefault != "" {
			crt, err := cs.matchSingleDomain(cs.certDefault, resolve)
			if err != nil {
				return nil, err
			}

			if crt != nil {
				return crt, nil
			}
		}

		return nil, nil
	})

	if err != nil {
		return nil, err
	}

	return crt.TLSCertificate()
}

func (cs *Storage) Match(domain string, resolve func(domain string) (*tls.Certificate, error)) (*tls.Certificate, error) {
	crt, err := cs.matchDomain(domain, resolve)

	// If match failed, try default cert.
	if errors.Is(err, rediscache.ErrNil) && cs.certDefault != "" && domain != cs.certDefault {
		return cs.DefaultCert(resolve)
	}

	return crt, err
}

func (cs *Storage) InvalidateMatch(domain string) error {
	labels := strings.SplitN(domain, ".", 2)
	toplevelDomain := "." + strings.Join(labels[1:], ".")

	return cs.rc.FuncCacheInvalidate(matchKey, toplevelDomain)
}

func (cs *Storage) DefaultCert(resolve func(domain string) (*tls.Certificate, error)) (*tls.Certificate, error) {
	return cs.matchDomain(cs.certDefault, resolve)
}
