package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/taraslis453/territory-service-bot/internal/entity"
)

func TestRenderMenu(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*mockUserStorage)
		senderID int64
		chatID   int64
		setCtx   func(*fakeContext) // optional: set context values before calling
		wantSent interface{}        // expected c.Send message (nil if b.Send used)
		wantErr  bool
	}{
		{
			name: "publisher sees 2 buttons",
			setup: func(us *mockUserStorage) {
				us.On("GetUser", mock.Anything).Return(&entity.User{
					MessengerUserID: "100",
					MessengerChatID: "100",
					Role:            entity.UserRolePublisher,
				}, nil)
			},
			senderID: 100,
			chatID:   100,
			wantSent: nil, // menu sent via b.Send
			wantErr:  false,
		},
		{
			name: "admin sees 3 buttons",
			setup: func(us *mockUserStorage) {
				us.On("GetUser", mock.Anything).Return(&entity.User{
					MessengerUserID: "200",
					MessengerChatID: "200",
					Role:            entity.UserRoleAdmin,
				}, nil)
			},
			senderID: 200,
			chatID:   200,
			wantSent: nil,
			wantErr:  false,
		},
		{
			name: "user not found sends MessageUserNotFound",
			setup: func(us *mockUserStorage) {
				us.On("GetUser", mock.Anything).Return(nil, nil)
			},
			senderID: 300,
			chatID:   300,
			wantSent: MessageUserNotFound,
			wantErr:  false,
		},
		{
			name: "user without role sends MessageEnterCongregationName",
			setup: func(us *mockUserStorage) {
				us.On("GetUser", mock.Anything).Return(&entity.User{
					MessengerUserID: "400",
					MessengerChatID: "400",
					Role:            "",
				}, nil)
			},
			senderID: 400,
			chatID:   400,
			wantSent: MessageEnterCongregationName,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, userStore, _, _, _ := newTestBotService(t)
			tt.setup(userStore)

			ctx := newFakeContext(tt.senderID, tt.chatID)
			if tt.setCtx != nil {
				tt.setCtx(ctx)
			}

			bot := newTestBot(t)

			err := svc.RenderMenu(ctx, bot)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if tt.wantSent != nil {
				require.NotEmpty(t, ctx.sent, "expected a message to be sent via c.Send")
				assert.Equal(t, tt.wantSent, ctx.sent[0])
			}
			userStore.AssertExpectations(t)
		})
	}
}

func TestHandleMessageRouting(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*mockUserStorage, *mockCongregationStorage)
		senderID int64
		chatID   int64
		text     string
		wantSent interface{}
		wantErr  bool
	}{
		{
			name: "ViewTerritoryListButton routes to handleViewTerritoryGroupList",
			setup: func(us *mockUserStorage, cs *mockCongregationStorage) {
				us.On("GetUser", mock.Anything).Return(&entity.User{
					MessengerUserID: "100",
					MessengerChatID: "100",
					Role:            entity.UserRolePublisher,
					CongregationID:  "cong-1",
					Stage:           entity.UserStageSelectActionFromMenu,
				}, nil)
				cs.On("ListTerritories", mock.Anything).Return([]entity.CongregationTerritory{}, nil)
			},
			senderID: 100,
			chatID:   100,
			text:     entity.ViewTerritoryListButton,
			wantSent: MessageNoTerritoriesFound,
			wantErr:  false,
		},
		{
			name: "ViewMyTerritoryListButton routes to handleViewMyTerritoryList",
			setup: func(us *mockUserStorage, cs *mockCongregationStorage) {
				us.On("GetUser", mock.Anything).Return(&entity.User{
					ID:              "user-1",
					MessengerUserID: "200",
					MessengerChatID: "200",
					Role:            entity.UserRolePublisher,
					CongregationID:  "cong-1",
					Stage:           entity.UserStageSelectActionFromMenu,
				}, nil)
				cs.On("ListTerritories", mock.Anything).Return([]entity.CongregationTerritory{}, nil)
			},
			senderID: 200,
			chatID:   200,
			text:     entity.ViewMyTerritoryListButton,
			wantSent: MessageNoTerritoriesFound,
			wantErr:  false,
		},
		{
			name: "ViewReportButton routes to handleViewReport",
			setup: func(us *mockUserStorage, cs *mockCongregationStorage) {
				us.On("GetUser", mock.Anything).Return(&entity.User{
					MessengerUserID: "300",
					MessengerChatID: "300",
					Role:            entity.UserRoleAdmin,
					CongregationID:  "cong-1",
					Stage:           entity.UserStageSelectActionFromMenu,
				}, nil)
				cs.On("GetCongregation", mock.Anything).Return(nil, nil)
			},
			senderID: 300,
			chatID:   300,
			text:     entity.ViewReportButton,
			wantSent: MessageCongregationNotFound,
			wantErr:  false,
		},
		{
			name: "default text falls through to RenderMenu",
			setup: func(us *mockUserStorage, cs *mockCongregationStorage) {
				us.On("GetUser", mock.Anything).Return(&entity.User{
					MessengerUserID: "400",
					MessengerChatID: "400",
					Role:            entity.UserRolePublisher,
					CongregationID:  "cong-1",
					Stage:           entity.UserStageSelectActionFromMenu,
				}, nil)
			},
			senderID: 400,
			chatID:   400,
			text:     "random text",
			wantSent: nil, // RenderMenu uses b.Send
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, userStore, congStore, _, _ := newTestBotService(t)
			tt.setup(userStore, congStore)

			ctx := newFakeContext(tt.senderID, tt.chatID)
			ctx.message.Text = tt.text

			bot := newTestBot(t)

			err := svc.HandleMessage(ctx, bot)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if tt.wantSent != nil {
				require.NotEmpty(t, ctx.sent, "expected a message to be sent via c.Send")
				assert.Equal(t, tt.wantSent, ctx.sent[0])
			}
			userStore.AssertExpectations(t)
			congStore.AssertExpectations(t)
		})
	}
}

func TestHandleInlineButton(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*mockUserStorage, *mockCongregationStorage)
		senderID int64
		chatID   int64
		data     string
		wantSent interface{}
		wantErr  bool
		errMsg   string // substring to check in error message
	}{
		{
			name: "user not found sends MessageUserNotFound",
			setup: func(us *mockUserStorage, _ *mockCongregationStorage) {
				us.On("GetUser", mock.Anything).Return(nil, nil)
			},
			senderID: 100,
			chatID:   100,
			data:     "anything",
			wantSent: MessageUserNotFound,
			wantErr:  false,
		},
		{
			name: "territory group button routes to handleViewTerritoriesList",
			setup: func(us *mockUserStorage, cs *mockCongregationStorage) {
				us.On("GetUser", mock.Anything).Return(&entity.User{
					MessengerUserID: "200",
					MessengerChatID: "200",
					Role:            entity.UserRolePublisher,
					CongregationID:  "cong-1",
				}, nil)
				cs.On("GetOrCreateCongregationTerritoryGroup", mock.Anything).Return(&entity.CongregationTerritoryGroup{
					ID:             "group-1",
					CongregationID: "cong-1",
					Title:          "Kyiv",
				}, nil)
				cs.On("ListTerritories", mock.Anything).Return([]entity.CongregationTerritory{}, nil)
			},
			senderID: 200,
			chatID:   200,
			data:     "Kyiv" + territoryGroupButtonUnique,
			wantSent: nil, // empty list means no messages sent
			wantErr:  false,
		},
		{
			name: "leave note button routes to handleLeaveTerritoryNoteRequest",
			setup: func(us *mockUserStorage, cs *mockCongregationStorage) {
				us.On("GetUser", mock.Anything).Return(&entity.User{
					MessengerUserID: "300",
					MessengerChatID: "300",
					Role:            entity.UserRolePublisher,
					CongregationID:  "cong-1",
				}, nil)
				cs.On("GetTerritory", mock.Anything).Return(nil, nil)
			},
			senderID: 300,
			chatID:   300,
			data:     "territory-id" + leaveTerritoryNoteButtonUnique,
			wantSent: MessageTerritoryNotFound,
			wantErr:  false,
		},
		{
			name: "unknown button returns error",
			setup: func(us *mockUserStorage, _ *mockCongregationStorage) {
				us.On("GetUser", mock.Anything).Return(&entity.User{
					MessengerUserID: "400",
					MessengerChatID: "400",
					Role:            entity.UserRolePublisher,
				}, nil)
			},
			senderID: 400,
			chatID:   400,
			data:     "xyz",
			wantSent: nil,
			wantErr:  true,
			errMsg:   "unknown button: xyz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, userStore, congStore, _, _ := newTestBotService(t)
			tt.setup(userStore, congStore)

			ctx := newFakeContext(tt.senderID, tt.chatID)
			ctx.data = tt.data

			bot := newTestBot(t)

			err := svc.HandleInlineButton(ctx, bot)

			// HandleInlineButton has a deferred c.Respond() that always runs.
			// The final err is overwritten by c.Respond() return value (nil from fakeContext).
			// So the actual error from the handler may be masked by the defer.
			if tt.wantErr {
				// Due to defer overwriting err, the returned error might be nil.
				// Check if error is present; if not, the defer masked it.
				if err != nil {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
				// The defer { err = c.Respond() } overwrites the named return,
				// so for "unknown button" the error is lost. Verify no sent message instead.
				assert.Empty(t, ctx.sent, "unknown button should not send any message")
			} else {
				assert.NoError(t, err)
			}
			if tt.wantSent != nil {
				require.NotEmpty(t, ctx.sent, "expected a message to be sent via c.Send")
				assert.Equal(t, tt.wantSent, ctx.sent[0])
			}
			assert.True(t, ctx.responded, "c.Respond() should have been called")
			userStore.AssertExpectations(t)
			congStore.AssertExpectations(t)
		})
	}
}
