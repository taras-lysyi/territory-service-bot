package storage

import (
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/taraslis453/territory-service-bot/internal/entity"
	"github.com/taraslis453/territory-service-bot/internal/service"
	"github.com/taraslis453/territory-service-bot/pkg/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type congregationStorage struct {
	database.Database
}

var _ service.CongregationStorage = (*congregationStorage)(nil)

func NewCongregationStorage(database database.Database) *congregationStorage {
	return &congregationStorage{database}
}

func (r *congregationStorage) GetCongregation(filter *service.GetCongregationFilter) (*entity.Congregation, error) {
	stmt := r.Instance()
	if filter.ID != "" {
		stmt = stmt.Where(&entity.Congregation{ID: filter.ID})
	}
	if filter.Name != "" {
		stmt = stmt.Where(&entity.Congregation{Name: filter.Name})
	}

	congregation := entity.Congregation{}
	err := stmt.
		Take(&congregation).
		Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &congregation, nil
}

func (r *congregationStorage) GetOrCreateCongregationTerritoryGroup(options *service.GetOrCreateCongregationTerritoryGroupOptions) (*entity.CongregationTerritoryGroup, error) {
	fetch := func() (*entity.CongregationTerritoryGroup, error) {
		var g entity.CongregationTerritoryGroup
		err := r.Instance().
			Where(&entity.CongregationTerritoryGroup{CongregationID: options.CongregationID, Title: options.Title}).
			Take(&g).Error
		if err != nil {
			return nil, err
		}
		return &g, nil
	}

	if g, err := fetch(); !errors.Is(err, gorm.ErrRecordNotFound) {
		return g, err
	}

	// Group not found — try to create it.
	newGroup := entity.CongregationTerritoryGroup{
		CongregationID: options.CongregationID,
		Title:          options.Title,
	}
	if err := r.Instance().Create(&newGroup).Error; err != nil {
		// Unique constraint violation: a concurrent goroutine created it first — re-fetch.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			g, err := fetch()
			if err != nil {
				return nil, fmt.Errorf("failed to re-fetch territory group after conflict: %w", err)
			}
			return g, nil
		}
		return nil, fmt.Errorf("failed to create territory group: %w", err)
	}
	return &newGroup, nil
}

func (r *congregationStorage) CreateTerritory(territory *entity.CongregationTerritory) (*entity.CongregationTerritory, error) {
	err := r.Instance().Create(territory).Error
	if err != nil {
		return nil, fmt.Errorf("failed to create territory: %w", err)
	}

	err = r.Instance().
		Where(&entity.CongregationTerritory{Title: territory.Title}).
		Take(&territory).
		Error
	if err != nil {
		return nil, fmt.Errorf("failed to get created territory: %w", err)
	}

	return territory, nil
}

func (r *congregationStorage) GetTerritory(filter *service.GetTerritoryFilter) (*entity.CongregationTerritory, error) {
	stmt := r.Instance()
	if filter.CongregationID != "" {
		stmt = stmt.Where(&entity.CongregationTerritory{CongregationID: filter.CongregationID})
	}
	if filter.Title != "" {
		stmt = stmt.Where(&entity.CongregationTerritory{Title: filter.Title})
	}
	if filter.ID != "" {
		stmt = stmt.Where(&entity.CongregationTerritory{ID: filter.ID})
	}
	if filter.GroupID != "" {
		stmt = stmt.Where(&entity.CongregationTerritory{GroupID: filter.GroupID})
	}

	territory := entity.CongregationTerritory{}
	err := stmt.
		Preload(clause.Associations).
		Take(&territory).
		Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &territory, nil
}

func (r *congregationStorage) ListTerritories(filter *service.ListTerritoriesFilter) ([]entity.CongregationTerritory, error) {
	stmt := r.Instance()
	if filter.CongregationID != "" {
		stmt = stmt.Where(&entity.CongregationTerritory{CongregationID: filter.CongregationID})
	}
	if filter.GroupID != "" {
		stmt = stmt.Where(&entity.CongregationTerritory{GroupID: filter.GroupID})
	}
	if filter.Available != nil {
		if *filter.Available {
			stmt = stmt.Where("in_use_by_user_id IS NULL")
		} else {
			stmt = stmt.Where("in_use_by_user_id IS NOT NULL")
		}
	}
	if filter.SortBy != "" {
		stmt = stmt.Order(filter.SortBy)
	}
	if filter.InUseByUserID != "" {
		stmt = stmt.Where(&entity.CongregationTerritory{InUseByUserID: &filter.InUseByUserID})
	}

	var territories []entity.CongregationTerritory
	err := stmt.
		Preload(clause.Associations).
		Find(&territories).
		Error
	if err != nil {
		return nil, err
	}

	return territories, nil
}

func (r *congregationStorage) ListTerritoryGroups(filter *service.ListTerritoryGroupsFilter) ([]entity.CongregationTerritoryGroup, error) {
	stmt := r.Instance()
	if filter.CongregationID != "" {
		stmt = stmt.Where(&entity.CongregationTerritoryGroup{CongregationID: filter.CongregationID})
	}
	if len(filter.IDs) > 0 {
		stmt = stmt.Where("id IN (?)", filter.IDs)
	}

	var groups []entity.CongregationTerritoryGroup
	err := stmt.
		Order("title ASC").
		Find(&groups).
		Error
	if err != nil {
		return nil, err
	}

	return groups, nil
}

func (r *congregationStorage) UpdateTerritory(territory *entity.CongregationTerritory) (*entity.CongregationTerritory, error) {
	err := r.Instance().
		Session(&gorm.Session{FullSaveAssociations: true}).
		Save(territory).Error
	if err != nil {
		return nil, err
	}

	return territory, nil
}

func (r *congregationStorage) AddTerritoryNote(territoryNote *entity.CongregationTerritoryNote) (*entity.CongregationTerritoryNote, error) {
	err := r.Instance().Create(territoryNote).Error
	if err != nil {
		return nil, err
	}

	return territoryNote, nil
}
