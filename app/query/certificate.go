package query

import (
	"context"
	"fmt"

	"github.com/Syncano/acme-proxy/app/models"
	"github.com/Syncano/pkg-go/v2/database"
	"github.com/Syncano/pkg-go/v2/database/manager"
	"github.com/Syncano/pkg-go/v2/rediscache"
	"github.com/go-pg/pg/v9"
	"github.com/go-pg/pg/v9/orm"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CertManager represents Cert manager.
type CertManager struct {
	*manager.Manager
	c *rediscache.Cache
}

// NewCertManager creates and returns new Cert manager.
func (q *Factory) NewCertManager(ctx context.Context) *CertManager {
	return &CertManager{Manager: manager.NewManager(database.WrapContext(ctx, nil), q.db), c: q.c}
}

// OneByDomain outputs object filtered by domain.
func (m *CertManager) OneByDomain(o *models.Certificate) error {
	return manager.RequireOne(
		m.c.SimpleModelCache(m.DB(), o, fmt.Sprintf("d=%s", o.Domain), func() (interface{}, error) {
			return o, m.ByDomainsQ(o, []string{o.Domain}).Select()
		}),
	)
}

// ByDomains filters object filtered by domains (IN).
func (m *CertManager) ByDomainsQ(o interface{}, domains []string) *orm.Query {
	return m.Query(o).Where("domain IN (?)", pg.In(domains))
}

func (m *CertManager) ForRefreshQ(o interface{}, failureThreshold int) *orm.Query {
	return m.Query(o).Where(
		"failures < ? AND auto_refresh IS TRUE AND updated_at < NOW() - INTERVAL '5 minute' AND expires_at > NOW() + refresh_before_days * INTERVAL '1 day'", failureThreshold)
}

func (m *CertManager) ListQ(o interface{}, domains []string, expirationLte *timestamppb.Timestamp, status models.CertificateStatus) *orm.Query {
	q := m.Query(o)

	if len(domains) > 0 {
		q = q.Where("domain IN (?)", pg.In(domains))
	}

	if expirationLte != nil {
		q = q.Where("expires_at < ?", expirationLte.AsTime())
	}

	if status != 0 {
		q = q.Where("status = ?", status)
	}

	return q
}
