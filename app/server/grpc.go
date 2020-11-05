package server

import (
	"context"
	"fmt"

	"github.com/Syncano/acme-proxy/app/models"
	"github.com/Syncano/acme-proxy/app/settings"
	"github.com/Syncano/pkg-go/v2/database/manager"
	pb "github.com/Syncano/syncanoapis/gen/go/syncano/hosting/acme/v1"
	"github.com/go-pg/pg/v9"
	"go.uber.org/zap"
)

func (s *Server) Create(ctx context.Context, req *pb.CreateRequest) (*pb.CreateResponse, error) {
	log := s.log.Logger().With(zap.String("domain", req.GetDomain()))

	if req.GetDomain() == "" {
		return nil, fmt.Errorf("create failed, empty domain")
	}

	ctx, cancel := context.WithTimeout(context.Background(), settings.Server.DomainVerifyTimeout)

	// Check and verify doamin.
	if err := s.checkDomain(ctx, req.Domain); err != nil {
		log.With(zap.Error(err)).Warn("Domain checking failed")
		cancel()

		return &pb.CreateResponse{
			Certificate: &pb.Certificate{
				Domain: req.Domain,
				Status: pb.Status_INVALID_DOMAIN,
			},
		}, nil
	}

	if err := s.verifyDomain(ctx, req.Domain); err != nil {
		log.With(zap.Error(err)).Warn("Domain verification failed")
		cancel()

		return &pb.CreateResponse{
			Certificate: &pb.Certificate{
				Domain: req.Domain,
				Status: pb.Status_DOMAIN_VERIFICATION_FAILED,
			},
		}, nil
	}

	cancel()

	if !req.Wait {
		s.jobRunner.Go(func() {
			_, err := s.obtainCertificate(ctx, req)
			log.With(zap.Error(err)).Warn("Failed to obtain certificate with wait=false")
		})

		return nil, nil
	}

	crt, err := s.obtainCertificate(ctx, req)
	if err != nil {
		return nil, err
	}

	return &pb.CreateResponse{
		Certificate: crt.Proto(),
	}, nil
}

func (s *Server) obtainCertificate(ctx context.Context, req *pb.CreateRequest) (*models.Certificate, error) {
	acmeCrt, err := s.acme.Obtain(req.Domain)
	if err != nil {
		return nil, fmt.Errorf("create failed, obtain error: %w", err)
	}

	crt := &models.Certificate{
		AcmeUserID: s.user.ID,
	}

	err = crt.FromAcmeCertificate(acmeCrt)
	if err != nil {
		return nil, fmt.Errorf("create failed, convert error: %w", err)
	}

	if refresh := req.GetRefresh(); refresh != nil {
		crt.AutoRefresh = refresh.AutoRefresh
		crt.RefreshBeforeDays = int(refresh.RefreshBeforeDays)
	}

	err = s.qf.NewCertManager(ctx).Insert(crt)
	if err != nil {
		return nil, fmt.Errorf("create failed, insert failed: %w", err)
	}

	return crt, nil
}

func (s *Server) List(ctx context.Context, req *pb.ListRequest) (*pb.ListResponse, error) {
	var certs []*models.Certificate

	mgr := s.qf.NewCertManager(ctx)
	if err := mgr.ListQ(&certs, req.GetDomains(), req.GetExpirationLte(), models.CertificateStatus(req.Status)).Limit(settings.Server.CertListLimit).Select(); err != nil {
		return nil, err
	}

	res := &pb.ListResponse{
		Certificates: make([]*pb.Certificate, len(certs)),
	}

	for i, cert := range certs {
		res.Certificates[i] = cert.Proto()
	}

	return res, nil
}

func (s *Server) Get(ctx context.Context, req *pb.GetRequest) (*pb.GetResponse, error) {
	domain := req.GetDomain()

	if domain == "" {
		return nil, fmt.Errorf("delete failed, empty domain")
	}

	crt := &models.Certificate{Domain: domain}
	mgr := s.qf.NewCertManager(ctx)
	err := mgr.OneByDomain(crt)

	if err != nil {
		return nil, err
	}

	return &pb.GetResponse{
		Certificate: crt.Proto(),
	}, nil
}

func (s *Server) Refresh(ctx context.Context, req *pb.RefreshRequest) (*pb.RefreshResponse, error) {
	var certs []*models.Certificate

	mgr := s.qf.NewCertManager(ctx)
	q := mgr.ListQ(&certs, req.GetDomains(), req.GetExpirationLte(), models.CertificateStatus(req.Status)).Limit(settings.Server.CertListLimit)
	res := &pb.RefreshResponse{}

	success, failed, err := s.certRefresh(mgr, certs, q)
	if err != nil {
		return nil, err
	}

	res.Refreshed = make([]*pb.Certificate, len(success))
	res.Failed = make([]*pb.Certificate, len(failed))

	for i, cert := range success {
		res.Refreshed[i] = cert.Proto()
	}

	for i, cert := range failed {
		res.Failed[i] = cert.Proto()
	}

	return res, nil
}

func (s *Server) Delete(ctx context.Context, req *pb.DeleteRequest) (*pb.DeleteResponse, error) {
	domain := req.GetDomain()

	if domain == "" {
		return nil, fmt.Errorf("delete failed, empty domain")
	}

	crt := &models.Certificate{Domain: domain}
	mgr := s.qf.NewCertManager(ctx)
	q := mgr.ByDomainsQ(crt, []string{domain})

	err := mgr.RunInTransaction(func(*pg.Tx) error {
		err := manager.Lock(q)
		if err == pg.ErrNoRows {
			return nil
		}

		return mgr.Delete(crt)
	})
	if err != nil {
		return nil, err
	}

	err = s.cert.InvalidateMatch(crt.Domain)

	return &pb.DeleteResponse{}, err
}
