package repository

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestPutExternalTenantPostgresReplay(t *testing.T) {
	dsn := os.Getenv("WEKNORA_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("WEKNORA_TEST_POSTGRES_DSN is not configured")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	schemaName := fmt.Sprintf("weknora_external_tenant_%d", time.Now().UnixNano())
	require.NoError(t, db.Exec("CREATE SCHEMA "+schemaName).Error)
	require.NoError(t, db.Exec("SET search_path TO "+schemaName).Error)
	t.Cleanup(func() {
		_ = db.Exec("SET search_path TO public").Error
		_ = db.Exec("DROP SCHEMA " + schemaName + " CASCADE").Error
	})
	require.NoError(t, db.AutoMigrate(&types.Tenant{}, &types.StorageBackend{}))
	require.NoError(t, db.Exec("ALTER TABLE tenants DROP COLUMN external_ref").Error)
	migration, err := os.ReadFile("../../../migrations/versioned/000092_external_tenant_ref.up.sql")
	require.NoError(t, err)
	require.NoError(t, db.Exec(string(migration)).Error)
	repo := NewTenantRepository(db)
	ref, err := types.ParseExternalTenantRef("5aca407e-a7d3-596f-8c32-9678d41863d5")
	require.NoError(t, err)

	first, created, err := repo.PutExternalTenant(
		context.Background(),
		ref,
		&types.Tenant{Name: "Workspace", Business: "platform"},
		&types.StorageBackend{Name: "System Local", Provider: "local"},
	)
	require.NoError(t, err)
	require.True(t, created)
	replay, created, err := repo.PutExternalTenant(
		context.Background(),
		ref,
		&types.Tenant{Name: "Ignored replay name", Business: "platform"},
		&types.StorageBackend{Name: "System Local", Provider: "local"},
	)
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, first.ID, replay.ID)
	var tenantCount, backendCount int64
	require.NoError(t, db.Model(&types.Tenant{}).Where("external_ref = ?", ref.String()).Count(&tenantCount).Error)
	require.NoError(t, db.Model(&types.StorageBackend{}).Where("tenant_id = ?", first.ID).Count(&backendCount).Error)
	require.EqualValues(t, 1, tenantCount)
	require.EqualValues(t, 1, backendCount)
}
