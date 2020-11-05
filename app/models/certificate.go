package models

import (
	"context"
	"crypto"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"time"

	"github.com/Syncano/acme-proxy/pkg/acme"
	"github.com/Syncano/acme-proxy/pkg/cert"
	"github.com/Syncano/pkg-go/v2/database/fields"
	pb "github.com/Syncano/syncanoapis/gen/go/syncano/hosting/acme/v1"
	"github.com/go-acme/lego/v3/certcrypto"
	"github.com/go-acme/lego/v3/certificate"
	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/timestamp"
)

// CertificateStatus enum.
type CertificateStatus int

const (
	CertificateStatusUnknown CertificateStatus = iota
	CertificateStatusOK
	CertificateStatusInvalidDomain
	CertificateStatusDomainVerificationFailed
)

type Certificate struct {
	tableName struct{} `pg:"certificate"` // nolint

	ID         int
	AcmeUserID int
	Domain     string
	URL        string
	StableURL  string

	PrivateKey        []byte
	PrivateKeyType    cert.KeyType
	Certificate       [][]byte `pg:",array"`
	IssuerCertificate []byte
	CSR               []byte
	Status            CertificateStatus
	Failures          int

	AutoRefresh       bool
	RefreshBeforeDays int

	CreatedAt fields.Time
	UpdatedAt fields.Time
	ExpiresAt fields.Time
}

func (c *Certificate) String() string {
	return fmt.Sprintf("Certificate<ID=%d Domain=%q>", c.ID, c.Domain)
}

func (c *Certificate) BeforeUpdate(ctx context.Context) (context.Context, error) {
	c.UpdatedAt.Set(time.Now()) // nolint: errcheck
	return ctx, nil
}

func (c *Certificate) AcmeCertificate() (acme.Certificate, error) {
	var (
		privateKey          []byte
		crt, issuerCrt, csr []byte
	)

	if len(c.PrivateKey) > 0 {
		key, err := cert.ParsePrivateKeyByType(c.PrivateKey, c.PrivateKeyType)
		if err != nil {
			return nil, fmt.Errorf("parsing certificate private key error: %w", err)
		}

		privateKey = certcrypto.PEMEncode(key)
	}

	if len(c.Certificate) > 0 {
		for _, crtBytes := range c.Certificate {
			crt = append(crt, certcrypto.PEMEncode(certcrypto.DERCertificateBytes(crtBytes))...)
		}
	}

	if len(c.IssuerCertificate) > 0 {
		issuerCrt = certcrypto.PEMEncode(certcrypto.DERCertificateBytes(c.IssuerCertificate))
	}

	if len(c.CSR) > 0 {
		csr = certcrypto.PEMEncode(certcrypto.DERCertificateBytes(c.CSR))
	}

	return &certificate.Resource{
		Domain:            c.Domain,
		CertURL:           c.URL,
		CertStableURL:     c.StableURL,
		PrivateKey:        privateKey,
		Certificate:       crt,
		IssuerCertificate: issuerCrt,
		CSR:               csr,
	}, nil
}

func (c *Certificate) TLSCertificate() (*tls.Certificate, error) {
	var (
		privateKey crypto.PrivateKey
		err        error
	)

	if len(c.PrivateKey) > 0 {
		privateKey, err = cert.ParsePrivateKeyByType(c.PrivateKey, c.PrivateKeyType)
	}

	return &tls.Certificate{
		Certificate: c.Certificate,
		PrivateKey:  privateKey,
	}, err
}

func (c *Certificate) Proto() *pb.Certificate {
	var exp *timestamp.Timestamp

	if c.ExpiresAt.IsNull() {
		exp, _ = ptypes.TimestampProto(c.ExpiresAt.Get().(time.Time))
	}

	return &pb.Certificate{
		Domain:     c.Domain,
		Status:     pb.Status(c.Status),
		Expiration: exp,
		Refresh: &pb.Refresh{
			AutoRefresh:       c.AutoRefresh,
			RefreshBeforeDays: uint32(c.RefreshBeforeDays),
		},
	}
}

func (c *Certificate) FromAcmeCertificate(crt acme.Certificate) error {
	var (
		keyBytes  []byte
		keyType   cert.KeyType
		err       error
		expiresAt time.Time
	)

	// Parse certificates.
	certBytes := cert.DecodePEMToBytesArray(crt.Certificate)
	x509Cert, err := x509.ParseCertificate(certBytes[0])

	if len(certBytes) == 0 {
		return errors.New("no certificates were found while parsing the bundle")
	}

	if x509Cert.IsCA {
		return fmt.Errorf("certificate for %s bundle starts with a CA certificate", crt.Domain)
	}

	expiresAt = x509Cert.NotAfter

	// Parse private key.
	if len(crt.PrivateKey) > 0 {
		keyBytes, keyType, err = cert.MarshalPrivateKey(crt.PrivateKey)
	}

	now := time.Now()

	c.Domain = crt.Domain
	c.URL = crt.CertURL
	c.StableURL = crt.CertStableURL

	c.PrivateKey = keyBytes
	c.PrivateKeyType = keyType
	c.Certificate = certBytes
	c.IssuerCertificate = cert.DecodePEMToBytes(crt.IssuerCertificate)
	c.CSR = cert.DecodePEMToBytes(crt.CSR)
	c.Status = CertificateStatusOK

	c.CreatedAt = fields.NewTime(&now)
	c.ExpiresAt = fields.NewTime(expiresAt)

	return err
}
