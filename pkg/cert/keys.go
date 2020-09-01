package cert

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"fmt"
)

type KeyType int

const (
	KeyTypeUnknown KeyType = iota
	KeyTypeEC
	KeyTypePKCS1
)

func ParsePrivateKeyByType(der []byte, typ KeyType) (crypto.PrivateKey, error) {
	switch typ {
	case KeyTypeEC:
		return x509.ParseECPrivateKey(der)
	case KeyTypePKCS1:
		return x509.ParsePKCS1PrivateKey(der)
	case KeyTypeUnknown:
	}

	return nil, fmt.Errorf("unknown private key type")
}

func ParsePrivateKey(der []byte) (crypto.PrivateKey, KeyType, error) {
	if key, err := x509.ParsePKCS1PrivateKey(der); err == nil {
		return key, KeyTypePKCS1, nil
	}

	if key, err := x509.ParsePKCS8PrivateKey(der); err == nil {
		switch key := key.(type) {
		case *rsa.PrivateKey:
			return key, KeyTypePKCS1, nil
		case *ecdsa.PrivateKey:
			return key, KeyTypeEC, nil
		default:
			return nil, KeyTypeUnknown, fmt.Errorf("found unknown private key type in PKCS#8 wrapping")
		}
	}

	if key, err := x509.ParseECPrivateKey(der); err == nil {
		return key, KeyTypeEC, nil
	}

	return nil, KeyTypeUnknown, fmt.Errorf("failed to parse private key")
}

func MarshalPrivateKey(key crypto.PrivateKey) ([]byte, KeyType, error) {
	var (
		data []byte
		err  error
		typ  KeyType
	)

	switch k := key.(type) {
	case *ecdsa.PrivateKey:
		data, err = x509.MarshalECPrivateKey(k)
		typ = KeyTypeEC
	case *rsa.PrivateKey:
		data = x509.MarshalPKCS1PrivateKey(k)
		typ = KeyTypePKCS1
	}

	return data, typ, err
}
