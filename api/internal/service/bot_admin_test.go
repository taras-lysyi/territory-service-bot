package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/taraslis453/territory-service-bot/internal/entity"
	"github.com/taraslis453/territory-service-bot/pkg/database/datatypes"
	tb "gopkg.in/telebot.v3"
)

func TestHandleCongregationPublisherJoinRequest(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*mockUserStorage, *mockCongregationStorage, *mockChatStorage)
		ctx      func() *fakeContext
		options  handleCongregationPublisherJoinRequestOptions
		wantSent interface{}
		wantErr  bool
	}{
		{
			name: "congregation not found",
			setup: func(_ *mockUserStorage, cs *mockCongregationStorage, _ *mockChatStorage) {
				cs.On("GetCongregation", mock.Anything).Return(nil, nil)
			},
			ctx: func() *fakeContext {
				return newFakeContext(100, 100)
			},
			options: handleCongregationPublisherJoinRequestOptions{
				User:             &entity.User{ID: "user-1"},
				CongregationName: "Nonexistent",
			},
			wantSent: MessageCongregationNotFound,
		},
		{
			name: "no admins",
			setup: func(us *mockUserStorage, cs *mockCongregationStorage, _ *mockChatStorage) {
				cs.On("GetCongregation", mock.Anything).Return(&entity.Congregation{
					ID:   "cong-1",
					Name: "Test Congregation",
				}, nil)
				us.On("ListUsers", mock.Anything).Return([]entity.User{}, nil)
			},
			ctx: func() *fakeContext {
				return newFakeContext(100, 100)
			},
			options: handleCongregationPublisherJoinRequestOptions{
				User:             &entity.User{ID: "user-1"},
				CongregationName: "Test Congregation",
			},
			wantSent: MessageCongregationAdminNotFound,
		},
		{
			name: "success sends join request to admin",
			setup: func(us *mockUserStorage, cs *mockCongregationStorage, chs *mockChatStorage) {
				cs.On("GetCongregation", mock.Anything).Return(&entity.Congregation{
					ID:   "cong-1",
					Name: "Test Congregation",
				}, nil)
				us.On("ListUsers", mock.Anything).Return([]entity.User{
					{
						ID:              "admin-1",
						MessengerChatID: "1",
						Role:            entity.UserRoleAdmin,
						CongregationID:  "cong-1",
					},
				}, nil)
				chs.On("CreateRequestActionState", mock.Anything).Return(&entity.RequestActionState{
					ID: "ras-1",
				}, nil)
				us.On("UpdateUser", mock.MatchedBy(func(u *entity.User) bool {
					return u.Stage == entity.UserPublisherStageWaitingForAdminApproval
				})).Return(&entity.User{
					ID:    "user-1",
					Stage: entity.UserPublisherStageWaitingForAdminApproval,
				}, nil)
			},
			ctx: func() *fakeContext {
				fc := newFakeContext(100, 100)
				fc.sender = &tb.User{ID: 100, FirstName: "Test", LastName: "User"}
				fc.message = &tb.Message{
					Sender: &tb.User{ID: 100, FirstName: "Test", LastName: "User"},
					Chat:   &tb.Chat{ID: 100},
				}
				return fc
			},
			options: handleCongregationPublisherJoinRequestOptions{
				User:             &entity.User{ID: "user-1"},
				CongregationName: "Test Congregation",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, userStore, congStore, chatStore, _ := newTestBotService(t)
			tt.setup(userStore, congStore, chatStore)

			bot := newTestBot(t)
			ctx := tt.ctx()

			err := svc.handleCongregationPublisherJoinRequest(ctx, bot, tt.options)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if tt.wantSent != nil {
				require.NotEmpty(t, ctx.sent, "expected a message to be sent")
				assert.Equal(t, tt.wantSent, ctx.sent[0])
			}

			// For the success case, verify the key mock calls.
			if tt.name == "success sends join request to admin" {
				chatStore.AssertCalled(t, "CreateRequestActionState", mock.Anything)
				userStore.AssertCalled(t, "UpdateUser", mock.MatchedBy(func(u *entity.User) bool {
					return u.Stage == entity.UserPublisherStageWaitingForAdminApproval
				}))
				// The success case sends confirmation via c.Send (captured in ctx.sent).
				require.NotEmpty(t, ctx.sent)
				assert.Equal(t, MessageCongregationJoinRequestSent("Test Congregation"), ctx.sent[0])
			}

			userStore.AssertExpectations(t)
			congStore.AssertExpectations(t)
			chatStore.AssertExpectations(t)
		})
	}
}

func TestHandleApprovePublisherJoinRequest(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*mockUserStorage, *mockChatStorage)
		wantSent interface{}
		wantErr  bool
	}{
		{
			name: "publisher not found",
			setup: func(us *mockUserStorage, _ *mockChatStorage) {
				us.On("GetUser", mock.Anything).Return(nil, nil)
			},
			wantSent: MessagePublisherNotFound,
		},
		{
			name: "success approves publisher",
			setup: func(us *mockUserStorage, chs *mockChatStorage) {
				publisher := &entity.User{
					ID:              "pub-1",
					MessengerUserID: "200",
					MessengerChatID: "200",
					FullName:        "Test Publisher",
				}
				// First GetUser call: lookup by ID (from handler).
				us.On("GetUser", mock.MatchedBy(func(f *GetUserFilter) bool {
					return f.ID == "pub-1"
				})).Return(publisher, nil)

				// UpdateUser for setting congregation/role/stage.
				us.On("UpdateUser", mock.MatchedBy(func(u *entity.User) bool {
					return u.ID == "pub-1" &&
						u.CongregationID == "cong-1" &&
						u.Role == entity.UserRolePublisher &&
						u.Stage == entity.UserStageSelectActionFromMenu
				})).Return(&entity.User{
					ID:              "pub-1",
					MessengerUserID: "200",
					MessengerChatID: "200",
					FullName:        "Test Publisher",
					CongregationID:  "cong-1",
					Role:            entity.UserRolePublisher,
					Stage:           entity.UserStageSelectActionFromMenu,
				}, nil)

				// RenderMenu calls GetUser by MessengerUserID.
				us.On("GetUser", mock.MatchedBy(func(f *GetUserFilter) bool {
					return f.MessengerUserID == "200"
				})).Return(&entity.User{
					ID:              "pub-1",
					MessengerUserID: "200",
					MessengerChatID: "200",
					FullName:        "Test Publisher",
					CongregationID:  "cong-1",
					Role:            entity.UserRolePublisher,
					Stage:           entity.UserStageSelectActionFromMenu,
				}, nil)

				chs.On("GetRequestActionState", "ras-1").Return(&entity.RequestActionState{
					ID: "ras-1",
					AdminMessages: datatypes.Slice[entity.AdminMessage]{
						{ChatID: "1", MessageID: "1"},
					},
				}, nil)
				chs.On("DeleteRequestActionState", "ras-1").Return(nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, userStore, _, chatStore, _ := newTestBotService(t)
			tt.setup(userStore, chatStore)

			bot := newTestBot(t)
			ctx := newFakeContext(1, 1)

			admin := &entity.User{
				ID:             "admin-1",
				CongregationID: "cong-1",
				Role:           entity.UserRoleAdmin,
			}

			err := svc.handleApprovePublisherJoinRequest(ctx, bot, admin, "pub-1", "ras-1")

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if tt.wantSent != nil {
				require.NotEmpty(t, ctx.sent, "expected a message to be sent")
				assert.Equal(t, tt.wantSent, ctx.sent[0])
			}

			userStore.AssertExpectations(t)
			chatStore.AssertExpectations(t)
		})
	}
}

func TestHandleRejectPublisherJoinRequest(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*mockUserStorage, *mockChatStorage)
		wantSent interface{}
		wantErr  bool
	}{
		{
			name: "publisher not found",
			setup: func(us *mockUserStorage, _ *mockChatStorage) {
				us.On("GetUser", mock.Anything).Return(nil, nil)
			},
			wantSent: MessagePublisherNotFound,
		},
		{
			name: "success rejects publisher",
			setup: func(us *mockUserStorage, chs *mockChatStorage) {
				publisher := &entity.User{
					ID:              "pub-1",
					MessengerUserID: "200",
					MessengerChatID: "200",
					FullName:        "Test Publisher",
				}
				us.On("GetUser", mock.Anything).Return(publisher, nil)
				us.On("UpdateUser", mock.MatchedBy(func(u *entity.User) bool {
					return u.Stage == entity.UserPublisherStageCongregationJoinRequestRejected
				})).Return(&entity.User{
					ID:              "pub-1",
					MessengerUserID: "200",
					MessengerChatID: "200",
					FullName:        "Test Publisher",
					Stage:           entity.UserPublisherStageCongregationJoinRequestRejected,
				}, nil)

				chs.On("GetRequestActionState", "ras-1").Return(&entity.RequestActionState{
					ID: "ras-1",
					AdminMessages: datatypes.Slice[entity.AdminMessage]{
						{ChatID: "1", MessageID: "1"},
					},
				}, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, userStore, _, chatStore, _ := newTestBotService(t)
			tt.setup(userStore, chatStore)

			bot := newTestBot(t)
			ctx := newFakeContext(1, 1)

			admin := &entity.User{
				ID:             "admin-1",
				CongregationID: "cong-1",
				Role:           entity.UserRoleAdmin,
			}

			err := svc.handleRejectPublisherJoinRequest(ctx, bot, admin, "pub-1", "ras-1")

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if tt.wantSent != nil {
				require.NotEmpty(t, ctx.sent, "expected a message to be sent")
				assert.Equal(t, tt.wantSent, ctx.sent[0])
			}

			userStore.AssertExpectations(t)
			chatStore.AssertExpectations(t)
		})
	}
}

func TestHandleApproveTerritoryTakeRequest(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*mockUserStorage, *mockCongregationStorage, *mockChatStorage)
		wantSent interface{}
		wantErr  bool
	}{
		{
			name: "publisher not found",
			setup: func(us *mockUserStorage, _ *mockCongregationStorage, _ *mockChatStorage) {
				us.On("GetUser", mock.Anything).Return(nil, nil)
			},
			wantSent: MessagePublisherNotFound,
		},
		{
			name: "territory not found",
			setup: func(us *mockUserStorage, cs *mockCongregationStorage, _ *mockChatStorage) {
				us.On("GetUser", mock.Anything).Return(&entity.User{
					ID:       "pub-1",
					FullName: "Test Publisher",
				}, nil)
				cs.On("GetTerritory", mock.Anything).Return(nil, nil)
			},
			wantSent: MessageTerritoryNotFound,
		},
		{
			name: "territory already taken",
			setup: func(us *mockUserStorage, cs *mockCongregationStorage, _ *mockChatStorage) {
				us.On("GetUser", mock.Anything).Return(&entity.User{
					ID:       "pub-1",
					FullName: "Test Publisher",
				}, nil)
				takenBy := "other-user"
				cs.On("GetTerritory", mock.Anything).Return(&entity.CongregationTerritory{
					ID:            "terr-1",
					Title:         "Territory 1",
					InUseByUserID: &takenBy,
				}, nil)
			},
			wantSent: MessageTerritoryNotAvailable,
		},
		{
			name: "success approves territory take",
			setup: func(us *mockUserStorage, cs *mockCongregationStorage, chs *mockChatStorage) {
				us.On("GetUser", mock.Anything).Return(&entity.User{
					ID:              "pub-1",
					MessengerUserID: "200",
					MessengerChatID: "200",
					FullName:        "Test Publisher",
				}, nil)
				cs.On("GetTerritory", mock.Anything).Return(&entity.CongregationTerritory{
					ID:             "terr-1",
					Title:          "Territory 1",
					CongregationID: "cong-1",
					GroupID:        "group-1",
				}, nil)
				cs.On("UpdateTerritory", mock.MatchedBy(func(t *entity.CongregationTerritory) bool {
					return t.InUseByUserID != nil && *t.InUseByUserID == "pub-1"
				})).Return(&entity.CongregationTerritory{
					ID:             "terr-1",
					Title:          "Territory 1",
					CongregationID: "cong-1",
					GroupID:        "group-1",
					InUseByUserID:  strPtr("pub-1"),
					LastTakenAt:    time.Now(),
				}, nil)
				// Goroutine calls: GetCongregation, ListTerritoryGroups, RecordTerritoryTake.
				cs.On("GetCongregation", mock.Anything).Return(&entity.Congregation{
					ID:            "cong-1",
					SpreadsheetID: "",
				}, nil)
				chs.On("GetRequestActionState", "ras-1").Return(&entity.RequestActionState{
					ID: "ras-1",
					AdminMessages: datatypes.Slice[entity.AdminMessage]{
						{ChatID: "1", MessageID: "1"},
					},
				}, nil)
				chs.On("DeleteRequestActionState", "ras-1").Return(nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, userStore, congStore, chatStore, _ := newTestBotService(t)
			tt.setup(userStore, congStore, chatStore)

			bot := newTestBot(t)
			ctx := newFakeContext(1, 1)

			err := svc.handleApproveTerritoryTakeRequest(ctx, bot, "pub-1", "terr-1", "ras-1")

			// Allow background goroutine (sheets recording) to finish.
			time.Sleep(50 * time.Millisecond)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if tt.wantSent != nil {
				require.NotEmpty(t, ctx.sent, "expected a message to be sent")
				assert.Equal(t, tt.wantSent, ctx.sent[0])
			}

			userStore.AssertExpectations(t)
			congStore.AssertExpectations(t)
			chatStore.AssertExpectations(t)
		})
	}
}

func TestHandleRejectTerritoryTakeRequest(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*mockUserStorage, *mockCongregationStorage, *mockChatStorage)
		wantSent interface{}
		wantErr  bool
	}{
		{
			name: "publisher not found",
			setup: func(us *mockUserStorage, _ *mockCongregationStorage, _ *mockChatStorage) {
				us.On("GetUser", mock.Anything).Return(nil, nil)
			},
			wantSent: MessagePublisherNotFound,
		},
		{
			name: "success rejects territory take",
			setup: func(us *mockUserStorage, cs *mockCongregationStorage, chs *mockChatStorage) {
				us.On("GetUser", mock.Anything).Return(&entity.User{
					ID:              "pub-1",
					MessengerUserID: "200",
					MessengerChatID: "200",
					FullName:        "Test Publisher",
				}, nil)
				cs.On("GetTerritory", mock.Anything).Return(&entity.CongregationTerritory{
					ID:    "terr-1",
					Title: "Territory 1",
				}, nil)
				chs.On("GetRequestActionState", "ras-1").Return(&entity.RequestActionState{
					ID: "ras-1",
					AdminMessages: datatypes.Slice[entity.AdminMessage]{
						{ChatID: "1", MessageID: "1"},
					},
				}, nil)
				chs.On("DeleteRequestActionState", "ras-1").Return(nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, userStore, congStore, chatStore, _ := newTestBotService(t)
			tt.setup(userStore, congStore, chatStore)

			bot := newTestBot(t)
			ctx := newFakeContext(1, 1)

			err := svc.handleRejectTerritoryTakeRequest(ctx, bot, "pub-1", "terr-1", "ras-1")

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if tt.wantSent != nil {
				require.NotEmpty(t, ctx.sent, "expected a message to be sent")
				assert.Equal(t, tt.wantSent, ctx.sent[0])
			}

			userStore.AssertExpectations(t)
			congStore.AssertExpectations(t)
			chatStore.AssertExpectations(t)
		})
	}
}

func strPtr(s string) *string {
	return &s
}
