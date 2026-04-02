package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/taraslis453/territory-service-bot/internal/entity"
	tb "gopkg.in/telebot.v3"
)

func TestHandleViewTerritoryGroupList(t *testing.T) {
	tests := []struct {
		name             string
		setup            func(*mockUserStorage, *mockCongregationStorage)
		user             *entity.User
		wantSent         interface{}
		wantSentCount    int
		wantAvailableNil *bool // nil = don't check, non-nil = check Available field in ListTerritories call
		wantErr          bool
	}{
		{
			name: "no territories sends MessageNoTerritoriesFound",
			setup: func(_ *mockUserStorage, cs *mockCongregationStorage) {
				cs.On("ListTerritories", mock.Anything).Return([]entity.CongregationTerritory{}, nil)
			},
			user: &entity.User{
				CongregationID: "cong-1",
				Role:           entity.UserRolePublisher,
			},
			wantSent:      MessageNoTerritoriesFound,
			wantSentCount: 1,
		},
		{
			name: "territories with groups sends MessageTerritoryList",
			setup: func(_ *mockUserStorage, cs *mockCongregationStorage) {
				cs.On("ListTerritories", mock.Anything).Return([]entity.CongregationTerritory{
					{ID: "t-1", GroupID: "g-1", Title: "Territory 1"},
					{ID: "t-2", GroupID: "g-1", Title: "Territory 2"},
					{ID: "t-3", GroupID: "g-2", Title: "Territory 3"},
				}, nil)
				cs.On("ListTerritoryGroups", mock.Anything).Return([]entity.CongregationTerritoryGroup{
					{ID: "g-1", Title: "Kyiv"},
					{ID: "g-2", Title: "Lviv"},
				}, nil)
			},
			user: &entity.User{
				CongregationID: "cong-1",
				Role:           entity.UserRolePublisher,
			},
			wantSent:      MessageTerritoryList,
			wantSentCount: 1,
		},
		{
			name: "admin sees all territories with Available nil",
			setup: func(_ *mockUserStorage, cs *mockCongregationStorage) {
				cs.On("ListTerritories", mock.MatchedBy(func(f *ListTerritoriesFilter) bool {
					return f.Available == nil
				})).Return([]entity.CongregationTerritory{
					{ID: "t-1", GroupID: "g-1", Title: "Territory 1"},
				}, nil)
				cs.On("ListTerritoryGroups", mock.Anything).Return([]entity.CongregationTerritoryGroup{
					{ID: "g-1", Title: "Kyiv"},
				}, nil)
			},
			user: &entity.User{
				CongregationID: "cong-1",
				Role:           entity.UserRoleAdmin,
			},
			wantSent:      MessageTerritoryList,
			wantSentCount: 1,
		},
		{
			name: "publisher sees available only with Available true",
			setup: func(_ *mockUserStorage, cs *mockCongregationStorage) {
				cs.On("ListTerritories", mock.MatchedBy(func(f *ListTerritoriesFilter) bool {
					return f.Available != nil && *f.Available == true
				})).Return([]entity.CongregationTerritory{
					{ID: "t-1", GroupID: "g-1", Title: "Territory 1"},
				}, nil)
				cs.On("ListTerritoryGroups", mock.Anything).Return([]entity.CongregationTerritoryGroup{
					{ID: "g-1", Title: "Kyiv"},
				}, nil)
			},
			user: &entity.User{
				CongregationID: "cong-1",
				Role:           entity.UserRolePublisher,
			},
			wantSent:      MessageTerritoryList,
			wantSentCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, userStore, congStore, _, _ := newTestBotService(t)
			tt.setup(userStore, congStore)

			ctx := newFakeContext(1, 1)

			err := svc.handleViewTerritoryGroupList(ctx, tt.user)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if tt.wantSent != nil {
				require.Len(t, ctx.sent, tt.wantSentCount, "unexpected number of sent messages")
				assert.Equal(t, tt.wantSent, ctx.sent[0])
			}
			congStore.AssertExpectations(t)
		})
	}
}

func TestHandleViewTerritoriesList(t *testing.T) {
	ptrStr := func(s string) *string { return &s }

	tests := []struct {
		name          string
		setup         func(*mockUserStorage, *mockCongregationStorage)
		user          *entity.User
		groupName     string
		wantSentCount int
		wantErr       bool
		assertions    func(t *testing.T, ctx *fakeContext)
	}{
		{
			name: "publisher sees territories with take button",
			setup: func(us *mockUserStorage, cs *mockCongregationStorage) {
				cs.On("GetOrCreateCongregationTerritoryGroup", mock.Anything).Return(&entity.CongregationTerritoryGroup{
					ID:    "g-1",
					Title: "Kyiv",
				}, nil)
				cs.On("ListTerritories", mock.Anything).Return([]entity.CongregationTerritory{
					{
						ID:       "t-1",
						GroupID:  "g-1",
						Title:    "Territory 1",
						FileID:   "file-1",
						FileType: entity.CongregationTerritoryFileTypePhoto,
					},
					{
						ID:       "t-2",
						GroupID:  "g-1",
						Title:    "Territory 2",
						FileID:   "file-2",
						FileType: entity.CongregationTerritoryFileTypeDocument,
					},
				}, nil)
				// No in-use territories, so ListUsers should not be called.
			},
			user: &entity.User{
				ID:             "user-1",
				CongregationID: "cong-1",
				Role:           entity.UserRolePublisher,
			},
			groupName:     "Kyiv",
			wantSentCount: 2,
			assertions: func(t *testing.T, ctx *fakeContext) {
				// First territory is a photo.
				photo, ok := ctx.sent[0].(*tb.Photo)
				require.True(t, ok, "expected *tb.Photo, got %T", ctx.sent[0])
				assert.Equal(t, "file-1", photo.FileID)
				assert.Contains(t, photo.Caption, "Territory 1")

				// Second territory is a document.
				doc, ok := ctx.sent[1].(*tb.Document)
				require.True(t, ok, "expected *tb.Document, got %T", ctx.sent[1])
				assert.Equal(t, "file-2", doc.FileID)
				assert.Contains(t, doc.Caption, "Territory 2")
			},
		},
		{
			name: "admin sees in-use info in caption",
			setup: func(us *mockUserStorage, cs *mockCongregationStorage) {
				cs.On("GetOrCreateCongregationTerritoryGroup", mock.Anything).Return(&entity.CongregationTerritoryGroup{
					ID:    "g-1",
					Title: "Kyiv",
				}, nil)
				cs.On("ListTerritories", mock.Anything).Return([]entity.CongregationTerritory{
					{
						ID:            "t-1",
						GroupID:       "g-1",
						Title:         "Territory 1",
						FileID:        "file-1",
						FileType:      entity.CongregationTerritoryFileTypePhoto,
						InUseByUserID: ptrStr("user-2"),
						LastTakenAt:   time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC),
					},
				}, nil)
				us.On("ListUsers", mock.Anything).Return([]entity.User{
					{
						ID:       "user-2",
						FullName: "Ivan Petrov",
					},
				}, nil)
			},
			user: &entity.User{
				ID:             "user-1",
				CongregationID: "cong-1",
				Role:           entity.UserRoleAdmin,
			},
			groupName:     "Kyiv",
			wantSentCount: 1,
			assertions: func(t *testing.T, ctx *fakeContext) {
				photo, ok := ctx.sent[0].(*tb.Photo)
				require.True(t, ok, "expected *tb.Photo, got %T", ctx.sent[0])
				assert.Contains(t, photo.Caption, "Ivan Petrov")
			},
		},
		{
			name: "no territories in group sends nothing",
			setup: func(_ *mockUserStorage, cs *mockCongregationStorage) {
				cs.On("GetOrCreateCongregationTerritoryGroup", mock.Anything).Return(&entity.CongregationTerritoryGroup{
					ID:    "g-1",
					Title: "Kyiv",
				}, nil)
				cs.On("ListTerritories", mock.Anything).Return([]entity.CongregationTerritory{}, nil)
			},
			user: &entity.User{
				ID:             "user-1",
				CongregationID: "cong-1",
				Role:           entity.UserRolePublisher,
			},
			groupName:     "Kyiv",
			wantSentCount: 0,
			assertions: func(t *testing.T, ctx *fakeContext) {
				assert.Empty(t, ctx.sent, "expected no messages to be sent")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, userStore, congStore, _, _ := newTestBotService(t)
			tt.setup(userStore, congStore)

			ctx := newFakeContext(1, 1)

			err := svc.handleViewTerritoriesList(ctx, tt.user, tt.groupName)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if tt.assertions != nil {
				tt.assertions(t, ctx)
			}
			congStore.AssertExpectations(t)
			userStore.AssertExpectations(t)
		})
	}
}

func TestHandleViewMyTerritoryList(t *testing.T) {
	tests := []struct {
		name          string
		setup         func(*mockCongregationStorage)
		user          *entity.User
		wantSentCount int
		wantErr       bool
		assertions    func(t *testing.T, ctx *fakeContext)
	}{
		{
			name: "no territories sends MessageNoTerritoriesFound",
			setup: func(cs *mockCongregationStorage) {
				cs.On("ListTerritories", mock.Anything).Return([]entity.CongregationTerritory{}, nil)
			},
			user: &entity.User{
				ID:             "user-1",
				CongregationID: "cong-1",
				Role:           entity.UserRolePublisher,
			},
			wantSentCount: 1,
			assertions: func(t *testing.T, ctx *fakeContext) {
				require.Len(t, ctx.sent, 1)
				assert.Equal(t, MessageNoTerritoriesFound, ctx.sent[0])
			},
		},
		{
			name: "photo territory with notes sends tb.Photo with caption containing notes",
			setup: func(cs *mockCongregationStorage) {
				cs.On("ListTerritories", mock.Anything).Return([]entity.CongregationTerritory{
					{
						ID:          "t-1",
						Title:       "Territory 1",
						FileID:      "file-1",
						FileType:    entity.CongregationTerritoryFileTypePhoto,
						LastTakenAt: time.Date(2026, 3, 10, 0, 0, 0, 0, time.UTC),
						Notes: []entity.CongregationTerritoryNote{
							{Text: "Note 1"},
							{Text: "Note 2"},
						},
					},
				}, nil)
			},
			user: &entity.User{
				ID:             "user-1",
				CongregationID: "cong-1",
				Role:           entity.UserRolePublisher,
			},
			wantSentCount: 1,
			assertions: func(t *testing.T, ctx *fakeContext) {
				require.Len(t, ctx.sent, 1)
				photo, ok := ctx.sent[0].(*tb.Photo)
				require.True(t, ok, "expected *tb.Photo, got %T", ctx.sent[0])
				assert.Equal(t, "file-1", photo.FileID)
				assert.Contains(t, photo.Caption, "Territory 1")
				assert.Contains(t, photo.Caption, "10.03.2026")
				assert.Contains(t, photo.Caption, "Note 1")
				assert.Contains(t, photo.Caption, "Note 2")
			},
		},
		{
			name: "document territory sends tb.Document",
			setup: func(cs *mockCongregationStorage) {
				cs.On("ListTerritories", mock.Anything).Return([]entity.CongregationTerritory{
					{
						ID:          "t-2",
						Title:       "Territory 2",
						FileID:      "file-2",
						FileType:    entity.CongregationTerritoryFileTypeDocument,
						LastTakenAt: time.Date(2026, 2, 5, 0, 0, 0, 0, time.UTC),
					},
				}, nil)
			},
			user: &entity.User{
				ID:             "user-1",
				CongregationID: "cong-1",
				Role:           entity.UserRolePublisher,
			},
			wantSentCount: 1,
			assertions: func(t *testing.T, ctx *fakeContext) {
				require.Len(t, ctx.sent, 1)
				doc, ok := ctx.sent[0].(*tb.Document)
				require.True(t, ok, "expected *tb.Document, got %T", ctx.sent[0])
				assert.Equal(t, "file-2", doc.FileID)
				assert.Contains(t, doc.Caption, "Territory 2")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, congStore, _, _ := newTestBotService(t)
			tt.setup(congStore)

			ctx := newFakeContext(1, 1)

			err := svc.handleViewMyTerritoryList(ctx, tt.user)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if tt.assertions != nil {
				tt.assertions(t, ctx)
			}
			congStore.AssertExpectations(t)
		})
	}
}
