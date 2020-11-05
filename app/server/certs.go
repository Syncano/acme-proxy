package server

import (
	"context"
	"fmt"

	"github.com/Syncano/acme-proxy/app/models"
	"github.com/Syncano/acme-proxy/app/query"
	"github.com/Syncano/acme-proxy/app/settings"
	"github.com/Syncano/pkg-go/v2/database/manager"
	"github.com/go-pg/pg/v9"
	"github.com/go-pg/pg/v9/orm"
	"go.uber.org/zap"
)

func (s *Server) certAutoRefresh(ctx context.Context) (isDone bool, err error) {
	l := s.log.Logger()

	var certs []*models.Certificate

	mgr := s.qf.NewCertManager(ctx)
	q := mgr.ForRefreshQ(&certs, settings.Server.RefreshFailureThreshold).Order("id").Limit(settings.Server.AutoRefreshBatch)

	l.Debug("Starting auto refresh of certs")

	success, failed, err := s.certRefresh(mgr, certs, q)
	successDomains := make([]string, len(success))
	failedDomains := make([]string, len(failed))

	for i, crt := range success {
		successDomains[i] = crt.Domain
	}

	for i, crt := range failed {
		failedDomains[i] = crt.Domain
	}

	// Log success and failures.
	l = l.With(zap.Strings("success", successDomains))

	if len(failed) == 0 {
		l.Info("Refreshing done")
	} else {
		l.With(zap.Strings("failed", failedDomains)).Warn("Refreshing done with some refreshes failed")
	}

	return (len(success) + len(failed)) < settings.Server.AutoRefreshBatch, err
}

func (s *Server) certRefresh(mgr *query.CertManager, certs []*models.Certificate, q *orm.Query) (success, failed []*models.Certificate, err error) {
	err = mgr.RunInTransaction(func(*pg.Tx) error {
		err := manager.Lock(q)
		if err == pg.ErrNoRows {
			return nil
		}

		if err != nil {
			return err
		}

		for _, cert := range certs {
			ctx, cancel := context.WithTimeout(context.Background(), settings.Server.DomainVerifyTimeout)

			// Check and verify domain.
			if err = s.checkDomain(ctx, cert.Domain); err != nil {
				cert.Status = models.CertificateStatusInvalidDomain
			} else if err = s.verifyDomain(ctx, cert.Domain); err != nil {
				cert.Status = models.CertificateStatusDomainVerificationFailed
			}

			cancel()

			if err != nil {
				cert.Failures++
				failed = append(failed, cert)

				if err := mgr.Update(cert, "status", "failures"); err != nil {
					return err
				}

				continue
			}

			// Perform actual refresh after internal verification.
			acmeCert, err := cert.AcmeCertificate()
			if err != nil {
				return err
			}

			acmeCert, err = s.acme.Refresh(acmeCert)
			if err != nil {
				return err
			}

			err = cert.FromAcmeCertificate(acmeCert)
			if err != nil {
				return err
			}

			// Reset failures.
			cert.Failures = 0
			success = append(success, cert)
		}

		if len(certs) == 0 {
			return nil
		}

		return mgr.Update(certs)
	})
	if err != nil {
		return nil, nil, fmt.Errorf("refresh transaction failed: %w", err)
	}

	for _, suc := range success {
		err = s.cert.InvalidateMatch(suc.Domain)
		if err != nil {
			err = fmt.Errorf("invalidate match error: %w", err)
		}
	}

	return success, failed, err
}
