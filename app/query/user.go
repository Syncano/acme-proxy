package query

import (
	"context"
	"reflect"

	"github.com/Syncano/acme-proxy/app/models"
	"github.com/Syncano/pkg-go/v2/database"
	"github.com/Syncano/pkg-go/v2/database/manager"
	"github.com/go-pg/pg/v9/orm"
)

// UserManager represents User manager.
type UserManager struct {
	*Factory
	*manager.Manager
}

// NewUserManager creates and returns new User manager.
func (q *Factory) NewUserManager(ctx context.Context) *UserManager {
	return &UserManager{Factory: q, Manager: manager.NewManager(database.WrapContext(ctx, nil), q.db)}
}

func (m *UserManager) LockTable() error {
	var o *models.User
	_, err := m.DB().ExecContext(m.DBContext().Context(), "LOCK TABLE ?", orm.GetTable(reflect.TypeOf(o).Elem()).FullName)

	return err
}

func (m *UserManager) First(o *models.User) error {
	return m.Query(o).First()
}
