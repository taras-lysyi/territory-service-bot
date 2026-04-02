package storage_test

import (
	"os"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/taraslis453/territory-service-bot/internal/entity"
	"github.com/taraslis453/territory-service-bot/internal/service"
	"github.com/taraslis453/territory-service-bot/internal/storage"
	"github.com/taraslis453/territory-service-bot/pkg/database"
	"gorm.io/gorm"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	host := os.Getenv("TEST_DB_HOST")
	if host == "" {
		host = "localhost"
	}
	db, err := database.NewPostgreSQL(&database.PostgreSQLConfig{
		User:     "admin",
		Password: "postgres",
		Host:     host,
		Database: "api",
	})
	if err != nil {
		t.Skipf("staging DB not available (%v) — skipping integration test", err)
	}
	return db.Instance()
}

// testCongregationID returns the first congregation ID from the staging DB,
// or skips the test if none exist.
func testCongregationID(t *testing.T, db *gorm.DB) string {
	t.Helper()
	var cong entity.Congregation
	err := db.First(&cong).Error
	require.NoError(t, err, "need at least one congregation in staging DB")
	return cong.ID
}

// TestGetOrCreateCongregationTerritoryGroup_ConcurrentCreation verifies that
// N concurrent goroutines calling GetOrCreate for the same group title produce
// exactly one DB row (no duplicates) and all return the same ID.
func TestGetOrCreateCongregationTerritoryGroup_ConcurrentCreation(t *testing.T) {
	db := testDB(t)
	congID := testCongregationID(t, db)

	const title = "_race_test_group_concurrent"
	const n = 20

	// Cleanup before and after.
	db.Exec("DELETE FROM congregation_territory_groups WHERE title = ?", title)
	t.Cleanup(func() {
		db.Exec("DELETE FROM congregation_territory_groups WHERE title = ?", title)
	})

	store := storage.NewCongregationStorage(database.NewFromGORM(db))

	type result struct {
		group *entity.CongregationTerritoryGroup
		err   error
	}
	results := make([]result, n)
	start := make(chan struct{})
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			g, err := store.GetOrCreateCongregationTerritoryGroup(&service.GetOrCreateCongregationTerritoryGroupOptions{
				CongregationID: congID,
				Title:          title,
			})
			results[idx] = result{g, err}
		}(i)
	}

	close(start)
	wg.Wait()

	// All goroutines must succeed.
	for i, r := range results {
		require.NoError(t, r.err, "goroutine %d returned error", i)
		require.NotNil(t, r.group, "goroutine %d returned nil group", i)
	}

	// All must return the same group ID.
	firstID := results[0].group.ID
	for i, r := range results {
		assert.Equal(t, firstID, r.group.ID, "goroutine %d got different ID", i)
	}

	// Exactly one row in DB.
	var rows []entity.CongregationTerritoryGroup
	db.Where("congregation_id = ? AND title = ?", congID, title).Find(&rows)
	assert.Len(t, rows, 1, "expected exactly 1 group in DB, got %d (race not fixed)", len(rows))
}
