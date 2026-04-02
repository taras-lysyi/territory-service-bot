package service

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/taraslis453/territory-service-bot/internal/entity"
	"github.com/taraslis453/territory-service-bot/pkg/sheets"
	tb "gopkg.in/telebot.v3"
)

func TestHandleStart(t *testing.T) {
	tests := []struct {
		name            string
		setup           func(*mockUserStorage, *mockCongregationStorage)
		senderID        int64
		chatID          int64
		wantSent        interface{}
		wantErr         bool
		wantUpdateStage entity.UserStage // verify UpdateUser was called with this stage
	}{
		{
			name: "new user creates account and asks for full name",
			setup: func(us *mockUserStorage, _ *mockCongregationStorage) {
				us.On("GetUser", mock.Anything).Return(nil, nil)
				us.On("CreateUser", mock.Anything).Return(&entity.User{
					MessengerUserID: "100",
					Stage:           entity.UserPublisherStageEnterFullName,
				}, nil)
			},
			senderID: 100,
			chatID:   100,
			wantSent: MessageEnterFullName,
		},
		{
			name: "existing user with empty full name asks for full name",
			setup: func(us *mockUserStorage, _ *mockCongregationStorage) {
				us.On("GetUser", mock.Anything).Return(&entity.User{
					MessengerUserID: "200",
					FullName:        "",
					Stage:           entity.UserPublisherStageEnterFullName,
				}, nil)
			},
			senderID: 200,
			chatID:   200,
			wantSent: MessageEnterFullName,
		},
		{
			name: "existing user waiting for approval sees waiting message",
			setup: func(us *mockUserStorage, _ *mockCongregationStorage) {
				us.On("GetUser", mock.Anything).Return(&entity.User{
					MessengerUserID: "300",
					FullName:        "John Doe",
					Stage:           entity.UserPublisherStageEnterCongregationName,
				}, nil)
			},
			senderID: 300,
			chatID:   300,
			wantSent: MessageWaitingForAdminApproval,
		},
		{
			name: "registered user updates stage and renders menu",
			setup: func(us *mockUserStorage, _ *mockCongregationStorage) {
				// HandleStart's GetUser and RenderMenu's GetUser both match this.
				us.On("GetUser", mock.MatchedBy(func(f *GetUserFilter) bool {
					return f.MessengerUserID == "400"
				})).Return(&entity.User{
					MessengerUserID: "400",
					MessengerChatID: "400",
					FullName:        "Jane Doe",
					Role:            entity.UserRolePublisher,
					CongregationID:  "cong-1",
					Stage:           entity.UserStageSelectActionFromMenu,
				}, nil)
				us.On("UpdateUser", mock.Anything).Return(&entity.User{
					MessengerUserID: "400",
					MessengerChatID: "400",
					FullName:        "Jane Doe",
					Role:            entity.UserRolePublisher,
					CongregationID:  "cong-1",
					Stage:           entity.UserStageSelectActionFromMenu,
				}, nil)
			},
			senderID: 400,
			chatID:   400,
			// RenderMenu calls b.Send (not c.Send) which fails with offline bot.
			// We accept the error and verify UpdateUser was called with the right stage.
			wantSent:        nil,
			wantErr:         true,
			wantUpdateStage: entity.UserStageSelectActionFromMenu,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, userStore, congStore, _, _ := newTestBotService(t)
			tt.setup(userStore, congStore)

			ctx := newFakeContext(tt.senderID, tt.chatID)
			bot, err := tb.NewBot(tb.Settings{Offline: true})
			require.NoError(t, err)

			err = svc.HandleStart(ctx, bot)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			if tt.wantSent != nil {
				require.NotEmpty(t, ctx.sent, "expected a message to be sent")
				assert.Equal(t, tt.wantSent, ctx.sent[0])
			}
			if tt.wantUpdateStage != "" {
				userStore.AssertCalled(t, "UpdateUser", mock.MatchedBy(func(u *entity.User) bool {
					return u.Stage == tt.wantUpdateStage
				}))
			}
			userStore.AssertExpectations(t)
		})
	}
}

func TestHandleMessage(t *testing.T) {
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
			name: "unknown user creates account and asks for congregation",
			setup: func(us *mockUserStorage, _ *mockCongregationStorage) {
				us.On("GetUser", mock.Anything).Return(nil, nil)
				us.On("CreateUser", mock.Anything).Return(&entity.User{
					MessengerUserID: "100",
					Stage:           entity.UserPublisherStageEnterCongregationName,
				}, nil)
			},
			senderID: 100,
			chatID:   100,
			text:     "hello",
			wantSent: MessageEnterCongregationName,
		},
		{
			name: "user in EnterFullName stage saves name and asks for congregation",
			setup: func(us *mockUserStorage, _ *mockCongregationStorage) {
				us.On("GetUser", mock.Anything).Return(&entity.User{
					MessengerUserID: "200",
					Stage:           entity.UserPublisherStageEnterFullName,
				}, nil)
				us.On("UpdateUser", mock.MatchedBy(func(u *entity.User) bool {
					return u.FullName == "Ivan Petrov" &&
						u.Stage == entity.UserPublisherStageEnterCongregationName
				})).Return(&entity.User{
					MessengerUserID: "200",
					FullName:        "Ivan Petrov",
					Stage:           entity.UserPublisherStageEnterCongregationName,
				}, nil)
			},
			senderID: 200,
			chatID:   200,
			text:     "Ivan Petrov",
			wantSent: MessageEnterCongregationName,
		},
		{
			name: "user waiting for approval sees waiting message",
			setup: func(us *mockUserStorage, _ *mockCongregationStorage) {
				us.On("GetUser", mock.Anything).Return(&entity.User{
					MessengerUserID: "300",
					FullName:        "John Doe",
					Stage:           entity.UserPublisherStageWaitingForAdminApproval,
				}, nil)
			},
			senderID: 300,
			chatID:   300,
			text:     "anything",
			wantSent: MessageWaitingForAdminApproval,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, userStore, congStore, _, _ := newTestBotService(t)
			tt.setup(userStore, congStore)

			ctx := newFakeContext(tt.senderID, tt.chatID)
			ctx.message.Text = tt.text
			bot, err := tb.NewBot(tb.Settings{Offline: true})
			require.NoError(t, err)

			err = svc.HandleMessage(ctx, bot)

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
		})
	}
}

func TestHandleViewReport(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*mockUserStorage, *mockCongregationStorage)
		user     *entity.User
		wantSent interface{}
		wantErr  bool
	}{
		{
			name:  "non-admin user sees not-admin message",
			setup: func(_ *mockUserStorage, _ *mockCongregationStorage) {},
			user: &entity.User{
				Role: entity.UserRolePublisher,
			},
			wantSent: MessageUserIsNotAdmin,
		},
		{
			name: "admin with no spreadsheet sees not-configured message",
			setup: func(_ *mockUserStorage, cs *mockCongregationStorage) {
				cs.On("GetCongregation", mock.Anything).Return(&entity.Congregation{
					ID:            "cong-1",
					Name:          "Test Congregation",
					SpreadsheetID: "",
				}, nil)
			},
			user: &entity.User{
				Role:           entity.UserRoleAdmin,
				CongregationID: "cong-1",
			},
			wantSent: MessageReportNotConfigured,
		},
		{
			name: "admin with spreadsheet gets report link",
			setup: func(_ *mockUserStorage, cs *mockCongregationStorage) {
				cs.On("GetCongregation", mock.Anything).Return(&entity.Congregation{
					ID:            "cong-1",
					Name:          "Test Congregation",
					SpreadsheetID: "spreadsheet-123",
				}, nil)
			},
			user: &entity.User{
				Role:           entity.UserRoleAdmin,
				CongregationID: "cong-1",
			},
			wantSent: MessageReportLink(sheets.GetSpreadsheetLink("spreadsheet-123")),
		},
		{
			name: "admin with missing congregation sees not-found message",
			setup: func(_ *mockUserStorage, cs *mockCongregationStorage) {
				cs.On("GetCongregation", mock.Anything).Return(nil, nil)
			},
			user: &entity.User{
				Role:           entity.UserRoleAdmin,
				CongregationID: "cong-missing",
			},
			wantSent: MessageCongregationNotFound,
		},
		{
			name: "admin with congregation fetch error returns error",
			setup: func(_ *mockUserStorage, cs *mockCongregationStorage) {
				cs.On("GetCongregation", mock.Anything).Return(nil, fmt.Errorf("db connection lost"))
			},
			user: &entity.User{
				Role:           entity.UserRoleAdmin,
				CongregationID: "cong-1",
			},
			wantSent: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, userStore, congStore, _, _ := newTestBotService(t)
			tt.setup(userStore, congStore)

			ctx := newFakeContext(1, 1)

			err := svc.handleViewReport(ctx, tt.user)

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
		})
	}
}
