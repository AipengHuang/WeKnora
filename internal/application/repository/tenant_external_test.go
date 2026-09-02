package repository

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestPutExternalTenantIsAtomicAndIdempotent(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "external.db")+"?_busy_timeout=5000"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.Tenant{}, &types.StorageBackend{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(4)
	repo := NewTenantRepository(db)
	ref, err := types.ParseExternalTenantRef("7b4a9b8e-0b65-5bb4-a07f-dc712921f3b7")
	require.NoError(t, err)

	const callers = 8
	ids := make(chan uint64, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tenant := &types.Tenant{Name: "Workspace", Business: "platform"}
			backend := &types.StorageBackend{Name: "System Local", Provider: "local"}
			stored, _, err := repo.PutExternalTenant(context.Background(), ref, tenant, backend)
			if err != nil {
				errs <- err
				return
			}
			ids <- stored.ID
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	var firstID uint64
	for id := range ids {
		if firstID == 0 {
			firstID = id
		}
		require.Equal(t, firstID, id)
	}
	require.NotZero(t, firstID)
	var tenantCount, backendCount int64
	require.NoError(t, db.Model(&types.Tenant{}).Count(&tenantCount).Error)
	require.NoError(t, db.Model(&types.StorageBackend{}).Count(&backendCount).Error)
	require.EqualValues(t, 1, tenantCount)
	require.EqualValues(t, 1, backendCount)
	var stored types.Tenant
	require.NoError(t, db.First(&stored, firstID).Error)
	require.NotNil(t, stored.DefaultStorageBackendID)

	require.NoError(t, db.Delete(&stored).Error)
	_, _, err = repo.PutExternalTenant(
		context.Background(),
		ref,
		&types.Tenant{Name: "Workspace", Business: "platform"},
		&types.StorageBackend{Name: "System Local", Provider: "local"},
	)
	require.ErrorIs(t, err, types.ErrExternalTenantDeleted)
}

func TestPutExternalTenantRollsBackWhenBackendInsertFails(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "rollback.db")), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&types.Tenant{}, &types.StorageBackend{}))
	require.NoError(t, db.Create(&types.StorageBackend{
		ID: "fixed-backend", TenantID: 99, Name: "Existing", Provider: "local",
	}).Error)
	repo := NewTenantRepository(db)
	ref, err := types.ParseExternalTenantRef("c42d4e28-81f6-5f5e-b7f5-cf40f45aa864")
	require.NoError(t, err)
	_, _, err = repo.PutExternalTenant(
		context.Background(),
		ref,
		&types.Tenant{Name: "Workspace", Business: "platform"},
		&types.StorageBackend{ID: "fixed-backend", Name: "System Local", Provider: "local"},
	)
	require.Error(t, err)
	var tenantCount int64
	require.NoError(t, db.Model(&types.Tenant{}).Count(&tenantCount).Error)
	require.Zero(t, tenantCount)
	require.False(t, errors.Is(err, types.ErrExternalTenantDeleted))
}
