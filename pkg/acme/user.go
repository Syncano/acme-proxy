package acme

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"encoding/base64"
	"fmt"

	"github.com/go-acme/lego/v3/registration"
	"gopkg.in/square/go-jose.v2"
)

type User struct {
	Email        string
	Registration *registration.Resource
	Key          crypto.PrivateKey
	thumbprint   string
}

var (
	_ registration.User = (*User)(nil)
)

func (u *User) GetEmail() string {
	return u.Email
}
func (u *User) GetRegistration() *registration.Resource {
	return u.Registration
}
func (u *User) GetPrivateKey() crypto.PrivateKey {
	return u.Key
}

func (u *User) Thumbprint() (string, error) {
	if u.thumbprint != "" {
		return u.thumbprint, nil
	}

	var publicKey crypto.PublicKey
	switch k := u.Key.(type) {
	case *ecdsa.PrivateKey:
		publicKey = k.Public()
	case *rsa.PrivateKey:
		publicKey = k.Public()
	}

	jwk := &jose.JSONWebKey{Key: publicKey}

	thumbBytes, err := jwk.Thumbprint(crypto.SHA256)
	if err != nil {
		return "", fmt.Errorf("jwk thumbprint generation error: %w", err)
	}

	u.thumbprint = base64.RawURLEncoding.EncodeToString(thumbBytes)
	return u.thumbprint, nil
}
