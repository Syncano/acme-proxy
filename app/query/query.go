package query

import (
	"github.com/Syncano/acme-proxy/app/models"
	"github.com/Syncano/pkg-go/v2/database"
	"github.com/Syncano/pkg-go/v2/rediscache"
	"github.com/go-pg/pg/v9/orm"
)

type Factory struct {
	db *database.DB
	c  *rediscache.Cache
}

func NewFactory(db *database.DB, c *rediscache.Cache) *Factory {
	f := &Factory{
		db: db,
		c:  c,
	}

	for _, model := range []interface{}{
		(*models.Certificate)(nil),
		(*models.User)(nil),
	} {
		db.AddModelDeleteHook(model, f.cacheDeleteHook)
		db.AddModelSoftDeleteHook(model, f.cacheDeleteHook)
		db.AddModelSaveHook(model, f.cacheSaveHook)
	}

	return f
}

func (q *Factory) cacheSaveHook(c database.DBContext, db orm.DB, created bool, m interface{}) error {
	if created {
		return nil
	}

	q.c.ModelCacheInvalidate(db, m)

	return nil
}

func (q *Factory) cacheDeleteHook(c database.DBContext, db orm.DB, m interface{}) error {
	q.c.ModelCacheInvalidate(db, m)
	return nil
}

func (q *Factory) Database() *database.DB {
	return q.db
}
