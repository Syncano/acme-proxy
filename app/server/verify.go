package server

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/Syncano/pkg-go/v2/util"
)

const checkKeyLength = 8

func (s *Server) checkDomain(ctx context.Context, domain string) error {
	_, err := net.DefaultResolver.LookupHost(ctx, domain)
	return err
}

func (s *Server) verifyDomain(ctx context.Context, domain string) error {
	key := util.GenerateRandomString(checkKeyLength)

	resp, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://%s/.well-known/echo/%s/", domain, key), nil)
	if err != nil {
		return err
	}

	defer resp.Body.Close()

	buf := make([]byte, checkKeyLength)

	_, err = io.ReadFull(resp.Body, buf)
	if err != nil {
		return err
	}

	if strings.Compare(key, string(buf)) != 0 {
		return fmt.Errorf("keys mismatch")
	}

	return nil
}
