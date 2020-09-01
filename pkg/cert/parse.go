package cert

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var validPEMBeginning = []byte("-----BEGIN")

func LoadCertficate(raw []byte) (*tls.Certificate, error) {
	var (
		cert tls.Certificate
		err  error
	)

	for {
		block, rest := pem.Decode(raw)
		if block == nil {
			break
		}

		if block.Type == "CERTIFICATE" {
			cert.Certificate = append(cert.Certificate, block.Bytes)
		} else {
			cert.PrivateKey, _, err = ParsePrivateKey(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("failure reading private key from: %w", err)
			}
		}
		raw = rest
	}

	if len(cert.Certificate) == 0 {
		return nil, fmt.Errorf("no certificate found")
	} else if cert.PrivateKey == nil {
		return nil, fmt.Errorf("no private key found")
	}

	return &cert, nil
}

func getCertFiles(dir string) ([][]byte, error) {
	tree := make(map[string][]string)

	err := filepath.Walk(dir,
		func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			dir := filepath.Dir(path)

			t := tree[dir]
			if t == nil {
				tree[dir] = []string{path}
			}

			tree[dir] = append(t, path)

			return nil
		})
	if err != nil {
		return nil, err
	}

	// Load and combine files

	ret := make([][]byte, 0, len(tree))

	for _, files := range tree {
		var cur []byte

		for _, file := range files {
			raw, err := ioutil.ReadFile(file)
			if err != nil {
				return nil, err
			}

			if filepath.Ext(file) == ".pem" {
				ret = append(ret, raw)
				continue
			}

			if !bytes.HasPrefix(raw, validPEMBeginning) {
				continue
			}

			if len(cur) != 0 {
				cur = append(cur, '\n')
			}

			cur = append(cur, raw...)
		}

		ret = append(ret, cur)
	}

	return ret, nil
}

func LoadCertificateFromPEM(pemBytes []byte) (*tls.Certificate, string, error) {
	tlsCert, err := LoadCertficate(pemBytes)
	if err != nil {
		return nil, "", err
	}

	// Parse cert to key by SAN
	cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return nil, "", err
	}

	var SANs []string
	if cert.Subject.CommonName != "" {
		SANs = append(SANs, strings.ToLower(cert.Subject.CommonName))
	}
	if cert.DNSNames != nil {
		sort.Strings(cert.DNSNames)
		for _, dnsName := range cert.DNSNames {
			if dnsName != cert.Subject.CommonName {
				SANs = append(SANs, strings.ToLower(dnsName))
			}
		}
	}
	if cert.IPAddresses != nil {
		for _, ip := range cert.IPAddresses {
			if ip.String() != cert.Subject.CommonName {
				SANs = append(SANs, strings.ToLower(ip.String()))
			}
		}
	}

	certKey := strings.Join(SANs, ",")

	return tlsCert, certKey, nil
}

func LoadCertificatesFromPath(dir string) (map[string]*tls.Certificate, error) {
	pems, err := getCertFiles(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to scan certificate dir \"%s\": %w", dir, err)
	}

	ret := make(map[string]*tls.Certificate)

	for _, pem := range pems {
		tlsCert, certKey, err := LoadCertificateFromPEM(pem)
		if err != nil {
			return nil, err
		}

		ret[certKey] = tlsCert
	}

	return ret, nil
}

func DecodePEMToBytes(bundle []byte) []byte {
	block, _ := pem.Decode(bundle)
	if block != nil {
		return block.Bytes
	}

	return nil
}

func DecodePEMToBytesArray(bundle []byte) [][]byte {
	var (
		ret   [][]byte
		block *pem.Block
	)

	for {
		block, bundle = pem.Decode(bundle)
		if block == nil {
			break
		}

		ret = append(ret, block.Bytes)
	}

	return ret
}
