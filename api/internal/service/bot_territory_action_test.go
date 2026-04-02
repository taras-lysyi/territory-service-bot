package service

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/taraslis453/territory-service-bot/internal/entity"
	tb "gopkg.in/telebot.v3"
)

func TestHandleLeaveTerritoryNoteRequest(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*mockCongregationStorage, *mockUserStorage)
		user      *entity.User
		territory string
		wantSent  interface{}
		wantErr   bool
	}{
		{
			name: "territory found updates user stage and sends HTML message",
			setup: func(cs *mockCongregationStorage, us *mockUserStorage) {
				cs.On("GetTerritory", mock.Anything).Return(&entity.CongregationTerritory{
					ID:    "t-1",
					Title: "Territory 1",
				}, nil)
				us.On("UpdateUser", mock.MatchedBy(func(u *entity.User) bool {
					return u.Stage == entity.UserStageLeaveTerritoryNote
				})).Return(&entity.User{Stage: entity.UserStageLeaveTerritoryNote}, nil)
			},
			user:      &entity.User{ID: "user-1", Stage: entity.UserStageSelectActionFromMenu},
			territory: "t-1",
			// Send is called with an HTML-formatted string containing the territory ID.
			wantSent: fmt.Sprintf("<a href=\"tg://btn/%s\">\u200b</a> %s", "t-1", MessageLeaveTerritoryNote("Territory 1")),
		},
		{
			name: "territory not found sends MessageTerritoryNotFound",
			setup: func(cs *mockCongregationStorage, _ *mockUserStorage) {
				cs.On("GetTerritory", mock.Anything).Return(nil, nil)
			},
			user:      &entity.User{ID: "user-1"},
			territory: "t-missing",
			wantSent:  MessageTerritoryNotFound,
		},
		{
			name: "db error returns error",
			setup: func(cs *mockCongregationStorage, _ *mockUserStorage) {
				cs.On("GetTerritory", mock.Anything).Return(nil, fmt.Errorf("db down"))
			},
			user:      &entity.User{ID: "user-1"},
			territory: "t-1",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, userStore, congStore, _, _ := newTestBotService(t)
			tt.setup(congStore, userStore)

			ctx := newFakeContext(1, 1)

			err := svc.handleLeaveTerritoryNoteRequest(ctx, tt.user, tt.territory)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if tt.wantSent != nil {
				require.NotEmpty(t, ctx.sent, "expected a message to be sent")
				assert.Equal(t, tt.wantSent, ctx.sent[0])
			}
			congStore.AssertExpectations(t)
			userStore.AssertExpectations(t)
		})
	}
}

func TestHandleLeaveTerritoryNoteMessage(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*mockCongregationStorage, *mockUserStorage)
		user      *entity.User
		territory string
		note      string
		wantSent  interface{}
		wantErr   bool
	}{
		{
			name: "note saved successfully",
			setup: func(cs *mockCongregationStorage, us *mockUserStorage) {
				userID := "user-1"
				cs.On("GetTerritory", mock.Anything).Return(&entity.CongregationTerritory{
					ID:            "t-1",
					Title:         "Territory 1",
					InUseByUserID: &userID,
				}, nil)
				cs.On("AddTerritoryNote", mock.MatchedBy(func(n *entity.CongregationTerritoryNote) bool {
					return n.TerritoryID == "t-1" && n.UserID == "user-1" && n.Text == "looks good"
				})).Return(&entity.CongregationTerritoryNote{ID: "note-1"}, nil)
				us.On("UpdateUser", mock.MatchedBy(func(u *entity.User) bool {
					return u.Stage == entity.UserStageSelectActionFromMenu
				})).Return(&entity.User{Stage: entity.UserStageSelectActionFromMenu}, nil)
			},
			user:      &entity.User{ID: "user-1", Stage: entity.UserStageLeaveTerritoryNote},
			territory: "t-1",
			note:      "looks good",
			wantSent:  MessageTerritoryNoteSaved,
		},
		{
			name: "territory not found sends MessageTerritoryNotFound",
			setup: func(cs *mockCongregationStorage, _ *mockUserStorage) {
				cs.On("GetTerritory", mock.Anything).Return(nil, nil)
			},
			user:      &entity.User{ID: "user-1"},
			territory: "t-missing",
			note:      "some note",
			wantSent:  MessageTerritoryNotFound,
		},
		{
			name: "territory not in use sends MessageTerritoryNotInUse",
			setup: func(cs *mockCongregationStorage, _ *mockUserStorage) {
				cs.On("GetTerritory", mock.Anything).Return(&entity.CongregationTerritory{
					ID:            "t-1",
					InUseByUserID: nil,
				}, nil)
			},
			user:      &entity.User{ID: "user-1"},
			territory: "t-1",
			note:      "some note",
			wantSent:  MessageTerritoryNotInUse,
		},
		{
			name: "wrong user sends MessageTerritoryCannotLeaveNote",
			setup: func(cs *mockCongregationStorage, _ *mockUserStorage) {
				otherUser := "user-other"
				cs.On("GetTerritory", mock.Anything).Return(&entity.CongregationTerritory{
					ID:            "t-1",
					InUseByUserID: &otherUser,
				}, nil)
			},
			user:      &entity.User{ID: "user-1"},
			territory: "t-1",
			note:      "some note",
			wantSent:  MessageTerritoryCannotLeaveNote,
		},
		{
			name: "db error on AddTerritoryNote returns error",
			setup: func(cs *mockCongregationStorage, _ *mockUserStorage) {
				userID := "user-1"
				cs.On("GetTerritory", mock.Anything).Return(&entity.CongregationTerritory{
					ID:            "t-1",
					InUseByUserID: &userID,
				}, nil)
				cs.On("AddTerritoryNote", mock.Anything).Return(nil, fmt.Errorf("db error"))
			},
			user:      &entity.User{ID: "user-1"},
			territory: "t-1",
			note:      "some note",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, userStore, congStore, _, _ := newTestBotService(t)
			tt.setup(congStore, userStore)

			ctx := newFakeContext(1, 1)

			err := svc.handleLeaveTerritoryNoteMessage(ctx, tt.user, tt.territory, tt.note)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if tt.wantSent != nil {
				require.NotEmpty(t, ctx.sent, "expected a message to be sent")
				assert.Equal(t, tt.wantSent, ctx.sent[0])
			}
			congStore.AssertExpectations(t)
			userStore.AssertExpectations(t)
		})
	}
}

func TestHandleReturnTerritoryRequest(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*mockCongregationStorage, *mockUserStorage)
		user      *entity.User
		territory string
		wantSent  interface{}
		wantErr   bool
	}{
		{
			name: "publisher returns territory notifies admins and edits caption",
			setup: func(cs *mockCongregationStorage, us *mockUserStorage) {
				userID := "user-1"
				cs.On("GetTerritory", mock.Anything).Return(&entity.CongregationTerritory{
					ID:             "t-1",
					Title:          "Territory 1",
					CongregationID: "cong-1",
					GroupID:        "group-1",
					InUseByUserID:  &userID,
					LastTakenAt:    time.Now(),
				}, nil)
				cs.On("UpdateTerritory", mock.MatchedBy(func(t *entity.CongregationTerritory) bool {
					return t.ID == "t-1" && t.InUseByUserID == nil
				})).Return(&entity.CongregationTerritory{ID: "t-1"}, nil)
				us.On("ListUsers", mock.MatchedBy(func(f *ListUsersFilter) bool {
					return f.CongregationID == "cong-1" && f.Role == entity.UserRoleAdmin
				})).Return([]entity.User{
					{ID: "admin-1", MessengerChatID: "chat-admin-1"},
				}, nil)
				// Async goroutine calls — stub to prevent panic, use Maybe so assertions don't require them.
				cs.On("GetCongregation", mock.Anything).Return(nil, nil).Maybe()
				cs.On("ListTerritoryGroups", mock.Anything).Return(nil, nil).Maybe()
			},
			user: &entity.User{
				ID:             "user-1",
				FullName:       "John Doe",
				Role:           entity.UserRolePublisher,
				CongregationID: "cong-1",
			},
			territory: "t-1",
		},
		{
			name: "territory not found sends MessageTerritoryNotFound",
			setup: func(cs *mockCongregationStorage, _ *mockUserStorage) {
				cs.On("GetTerritory", mock.Anything).Return(nil, nil)
			},
			user: &entity.User{
				ID:   "user-1",
				Role: entity.UserRolePublisher,
			},
			territory: "t-missing",
			wantSent:  MessageTerritoryNotFound,
		},
		{
			name: "db error on GetTerritory returns error",
			setup: func(cs *mockCongregationStorage, _ *mockUserStorage) {
				cs.On("GetTerritory", mock.Anything).Return(nil, fmt.Errorf("db down"))
			},
			user: &entity.User{
				ID:   "user-1",
				Role: entity.UserRolePublisher,
			},
			territory: "t-1",
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, userStore, congStore, _, _ := newTestBotService(t)
			tt.setup(congStore, userStore)

			ctx := newFakeContext(1, 1)
			ctx.message.ID = 1
			ctx.message.Chat = &tb.Chat{ID: 1}
			ctx.message.Caption = "old caption"

			bot := newTestBot(t)

			err := svc.handleReturnTerritoryRequest(ctx, bot, tt.user, tt.territory)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if tt.wantSent != nil {
				require.NotEmpty(t, ctx.sent, "expected a message to be sent")
				assert.Equal(t, tt.wantSent, ctx.sent[0])
			}
			congStore.AssertExpectations(t)
			userStore.AssertExpectations(t)
		})
	}
}

func TestHandleTakeTerritoryRequest_DoubleClickCreatesDuplicateState(t *testing.T) {
	// Replicates the production bug: user clicks "Take Territory" twice while
	// the bot is slow (~500ms DB queries). Both calls see InUseByUserID==nil
	// (territory not yet assigned), so two RequestActionStates are created and
	// every admin receives duplicate messages.
	//
	// Expected: second call should detect a pending request and return early.
	// Actual (bug): CreateRequestActionState is called twice.
	svc, userStore, congStore, chatStore, _ := newTestBotService(t)

	// Territory stays unassigned — both calls see InUseByUserID == nil.
	congStore.On("GetTerritory", mock.Anything).Return(&entity.CongregationTerritory{
		ID:       "t-1",
		Title:    "Territory 1",
		FileID:   "file-123",
		FileType: entity.CongregationTerritoryFileTypePhoto,
	}, nil)
	userStore.On("ListUsers", mock.MatchedBy(func(f *ListUsersFilter) bool {
		return f.CongregationID == "cong-1" && f.Role == entity.UserRoleAdmin
	})).Return([]entity.User{
		{ID: "admin-1", MessengerChatID: "chat-admin-1"},
	}, nil)
	// First call: no existing pending state.
	chatStore.On("GetRequestActionStateByTerritoryID", "t-1").Return(nil, nil).Once()
	chatStore.On("CreateRequestActionState", mock.Anything).Return(&entity.RequestActionState{ID: "ras-1", TerritoryID: "t-1"}, nil)
	// Second call: state now exists — should short-circuit.
	chatStore.On("GetRequestActionStateByTerritoryID", "t-1").Return(&entity.RequestActionState{ID: "ras-1", TerritoryID: "t-1"}, nil).Once()

	user := &entity.User{
		ID:             "user-1",
		FullName:       "John Doe",
		CongregationID: "cong-1",
	}
	bot := newTestBot(t)

	makeCtx := func() *fakeContext {
		ctx := newFakeContext(1, 1)
		ctx.callback = &tb.Callback{Message: &tb.Message{ID: 1, Chat: &tb.Chat{ID: 1}}}
		return ctx
	}

	// First click — expected to succeed.
	require.NoError(t, svc.handleTakeTerritoryRequest(makeCtx(), bot, user, "t-1"))

	// Second click while bot was slow to respond.
	require.NoError(t, svc.handleTakeTerritoryRequest(makeCtx(), bot, user, "t-1"))

	// Fix verified: second call detects existing pending state and returns early.
	chatStore.AssertNumberOfCalls(t, "CreateRequestActionState", 1)
}

func TestHandleTakeTerritoryRequest(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(*mockCongregationStorage, *mockUserStorage, *mockChatStorage)
		user      *entity.User
		territory string
		wantSent  interface{}
		wantErr   bool
	}{
		{
			name: "territory not found sends MessageTerritoryNotFound",
			setup: func(cs *mockCongregationStorage, _ *mockUserStorage, _ *mockChatStorage) {
				cs.On("GetTerritory", mock.Anything).Return(nil, nil)
			},
			user:      &entity.User{ID: "user-1", CongregationID: "cong-1"},
			territory: "t-missing",
			wantSent:  MessageTerritoryNotFound,
		},
		{
			name: "territory unavailable sends MessageTerritoryNotAvailable",
			setup: func(cs *mockCongregationStorage, _ *mockUserStorage, _ *mockChatStorage) {
				otherUser := "user-other"
				cs.On("GetTerritory", mock.Anything).Return(&entity.CongregationTerritory{
					ID:            "t-1",
					InUseByUserID: &otherUser,
				}, nil)
			},
			user:      &entity.User{ID: "user-1", CongregationID: "cong-1"},
			territory: "t-1",
			wantSent:  MessageTerritoryNotAvailable,
		},
		{
			name: "request sent to admins and caption edited",
			setup: func(cs *mockCongregationStorage, us *mockUserStorage, chatStore *mockChatStorage) {
				cs.On("GetTerritory", mock.Anything).Return(&entity.CongregationTerritory{
					ID:       "t-1",
					Title:    "Territory 1",
					FileID:   "file-123",
					FileType: entity.CongregationTerritoryFileTypePhoto,
				}, nil)
				us.On("ListUsers", mock.MatchedBy(func(f *ListUsersFilter) bool {
					return f.CongregationID == "cong-1" && f.Role == entity.UserRoleAdmin
				})).Return([]entity.User{
					{ID: "admin-1", MessengerChatID: "chat-admin-1"},
				}, nil)
				chatStore.On("GetRequestActionStateByTerritoryID", "t-1").Return(nil, nil)
				chatStore.On("CreateRequestActionState", mock.Anything).Return(&entity.RequestActionState{
					ID: "ras-1",
				}, nil)
			},
			user: &entity.User{
				ID:             "user-1",
				FullName:       "John Doe",
				CongregationID: "cong-1",
			},
			territory: "t-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, userStore, congStore, chatStore, _ := newTestBotService(t)
			tt.setup(congStore, userStore, chatStore)

			ctx := newFakeContext(1, 1)
			ctx.callback = &tb.Callback{
				Message: &tb.Message{
					ID:   1,
					Chat: &tb.Chat{ID: 1},
				},
			}

			bot := newTestBot(t)

			err := svc.handleTakeTerritoryRequest(ctx, bot, tt.user, tt.territory)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if tt.wantSent != nil {
				require.NotEmpty(t, ctx.sent, "expected a message to be sent")
				assert.Equal(t, tt.wantSent, ctx.sent[0])
			}
			congStore.AssertExpectations(t)
			userStore.AssertExpectations(t)
			chatStore.AssertExpectations(t)
		})
	}
}
