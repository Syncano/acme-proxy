package acme

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"

	"github.com/Syncano/pkg-go/v2/log"
	"github.com/Syncano/pkg-go/v2/rediscache"
	"github.com/bsm/redislock"
	"github.com/go-acme/lego/v3/certcrypto"
	"github.com/go-acme/lego/v3/certificate"
	"github.com/go-acme/lego/v3/lego"
	"github.com/go-acme/lego/v3/registration"
)

type Client struct {
	user   *User
	config *lego.Config
	client *lego.Client

	rc     *rediscache.Cache
	locker *redislock.Client
	logger *log.Logger
}

func NewClient(cadir string, rc *rediscache.Cache, locker *redislock.Client, logger *log.Logger) *Client {
	config := lego.NewConfig(nil)
	config.CADirURL = cadir
	config.Certificate.KeyType = certcrypto.RSA2048

	ac := &Client{
		config: config,
		rc:     rc,
		locker: locker,
		logger: logger,
	}

	return ac
}

func (a *Client) User() *User {
	return a.user
}

func (a *Client) Client() *lego.Client {
	if a.client != nil {
		return a.client
	}

	var err error

	// A client facilitates communication with the CA server.
	a.client, err = lego.NewClient(a.config)
	if err != nil {
		panic(err)
	}

	err = a.client.Challenge.SetHTTP01Provider(NewHTTP01ProviderServer())
	if err != nil {
		panic(err)
	}

	return a.client
}

func (a *Client) InitializeUser(user *User) (*User, error) {
	if a.user != nil {
		return nil, fmt.Errorf("user already initialized")
	}

	if user == nil || user.Email == "" {
		return nil, fmt.Errorf("user has to be non-nil and needs to have an email")
	}

	// If User is already registered, save it.
	if user.Key != nil && user.Registration != nil {
		a.config.User = user
		a.user = user

		return user, nil
	}

	// Create a user. New accounts need an email and private key to start.
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	user.Key = privateKey

	// New users will need to register.
	a.config.User = user

	// A client facilitates communication with the CA server.
	reg, err := a.Client().Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
	if err != nil {
		return nil, err
	}
	user.Registration = reg

	a.user = user

	return user, nil
}

func (a *Client) Thumbprint() (string, error) {
	if a.user == nil {
		return "", fmt.Errorf("user has to be non-nil")
	}

	return a.user.Thumbprint()
}

type Certificate *certificate.Resource

func (a *Client) Obtain(domains ...string) (Certificate, error) {
	request := certificate.ObtainRequest{
		Domains: domains,
		Bundle:  true,
	}

	if len(domains) == 0 {
		return nil, fmt.Errorf("unable to obtain certificate for empty domains")
	}

	cert, err := a.Client().Certificate.Obtain(request)

	if err != nil {
		return nil, fmt.Errorf("unable to generate a certificate for domains %v: %w", domains, err)
	}

	if cert == nil {
		return nil, fmt.Errorf("domains %v do not generate a certificate", domains)
	}

	if len(cert.Certificate) == 0 || len(cert.PrivateKey) == 0 {
		return nil, fmt.Errorf("domains %v generate certificate with no value: %v", domains, cert)
	}

	return cert, err
}

func (a *Client) Refresh(certRes *certificate.Resource) (Certificate, error) {
	if certRes == nil {
		return nil, fmt.Errorf("unable to renew a nil certificate")
	}

	cert, err := a.Client().Certificate.Renew(*certRes, true, false)
	domain := certRes.Domain

	if err != nil {
		return nil, fmt.Errorf("unable to renew a certificate for domain %s: %w", domain, err)
	}

	if len(cert.Certificate) == 0 || len(cert.PrivateKey) == 0 {
		return nil, fmt.Errorf("domain %s generate certificate with no value: %v", domain, cert)
	}

	return cert, err
}
