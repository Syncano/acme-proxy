package models

import (
	"fmt"
	"time"

	"github.com/Syncano/acme-proxy/pkg/acme"
	"github.com/Syncano/acme-proxy/pkg/cert"
	"github.com/Syncano/pkg-go/v2/database/fields"
	jsoniter "github.com/json-iterator/go"
)

type User struct {
	tableName struct{} `pg:"acme_user"` // nolint

	ID             int
	Email          string
	PrivateKey     []byte
	PrivateKeyType cert.KeyType
	Registration   fields.JSONB

	CreatedAt fields.Time
}

func (m *User) String() string {
	return fmt.Sprintf("User<ID=%d Email=%q>", m.ID, m.Email)
}

func (m *User) AcmeUser() (*acme.User, error) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	key, err := cert.ParsePrivateKeyByType(m.PrivateKey, m.PrivateKeyType)
	if err != nil {
		return nil, fmt.Errorf("parsing user private key error: %w", err)
	}

	acmeUser := &acme.User{
		Email: m.Email,
		Key:   key,
	}

	err = json.Unmarshal(m.Registration.Bytes, &acmeUser.Registration)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling registration bytes error: %w", err)
	}

	return acmeUser, nil
}

func NewUserFromAcmeUser(u *acme.User) (*User, error) {
	var json = jsoniter.ConfigCompatibleWithStandardLibrary

	keyBytes, keyType, err := cert.MarshalPrivateKey(u.Key)
	if err != nil {
		return nil, fmt.Errorf("marshaling user private key error: %w", err)
	}

	regBytes, err := json.Marshal(u.Registration)
	if err != nil {
		return nil, fmt.Errorf("marshaling user registration bytes error: %w", err)
	}

	now := time.Now()

	return &User{
		Email:          u.Email,
		PrivateKey:     keyBytes,
		PrivateKeyType: keyType,
		Registration:   fields.NewJSONB(regBytes),
		CreatedAt:      fields.NewTime(&now),
	}, nil
}
