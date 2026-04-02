package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/taraslis453/territory-service-bot/config"
	"github.com/taraslis453/territory-service-bot/internal/entity"
	"github.com/taraslis453/territory-service-bot/pkg/logging"
	zapLib "go.uber.org/zap"
	tb "gopkg.in/telebot.v3"
)

// --- Storage Mocks ---

type mockUserStorage struct{ mock.Mock }

func (m *mockUserStorage) CreateUser(u *entity.User) (*entity.User, error) {
	args := m.Called(u)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.User), args.Error(1)
}

func (m *mockUserStorage) GetUser(f *GetUserFilter) (*entity.User, error) {
	args := m.Called(f)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.User), args.Error(1)
}

func (m *mockUserStorage) UpdateUser(u *entity.User) (*entity.User, error) {
	args := m.Called(u)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.User), args.Error(1)
}

func (m *mockUserStorage) ListUsers(f *ListUsersFilter) ([]entity.User, error) {
	args := m.Called(f)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]entity.User), args.Error(1)
}

type mockCongregationStorage struct{ mock.Mock }

func (m *mockCongregationStorage) GetCongregation(f *GetCongregationFilter) (*entity.Congregation, error) {
	args := m.Called(f)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.Congregation), args.Error(1)
}

func (m *mockCongregationStorage) GetOrCreateCongregationTerritoryGroup(o *GetOrCreateCongregationTerritoryGroupOptions) (*entity.CongregationTerritoryGroup, error) {
	args := m.Called(o)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.CongregationTerritoryGroup), args.Error(1)
}

func (m *mockCongregationStorage) CreateTerritory(t *entity.CongregationTerritory) (*entity.CongregationTerritory, error) {
	args := m.Called(t)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.CongregationTerritory), args.Error(1)
}

func (m *mockCongregationStorage) GetTerritory(f *GetTerritoryFilter) (*entity.CongregationTerritory, error) {
	args := m.Called(f)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.CongregationTerritory), args.Error(1)
}

func (m *mockCongregationStorage) ListTerritories(f *ListTerritoriesFilter) ([]entity.CongregationTerritory, error) {
	args := m.Called(f)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]entity.CongregationTerritory), args.Error(1)
}

func (m *mockCongregationStorage) ListTerritoryGroups(f *ListTerritoryGroupsFilter) ([]entity.CongregationTerritoryGroup, error) {
	args := m.Called(f)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]entity.CongregationTerritoryGroup), args.Error(1)
}

func (m *mockCongregationStorage) UpdateTerritory(t *entity.CongregationTerritory) (*entity.CongregationTerritory, error) {
	args := m.Called(t)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.CongregationTerritory), args.Error(1)
}

func (m *mockCongregationStorage) AddTerritoryNote(n *entity.CongregationTerritoryNote) (*entity.CongregationTerritoryNote, error) {
	args := m.Called(n)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.CongregationTerritoryNote), args.Error(1)
}

type mockChatStorage struct{ mock.Mock }

func (m *mockChatStorage) CreateRequestActionState(r *entity.RequestActionState) (*entity.RequestActionState, error) {
	args := m.Called(r)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.RequestActionState), args.Error(1)
}

func (m *mockChatStorage) GetRequestActionState(id string) (*entity.RequestActionState, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.RequestActionState), args.Error(1)
}

func (m *mockChatStorage) DeleteRequestActionState(id string) error {
	args := m.Called(id)
	return args.Error(0)
}

func (m *mockChatStorage) GetRequestActionStateByTerritoryID(territoryID string) (*entity.RequestActionState, error) {
	args := m.Called(territoryID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*entity.RequestActionState), args.Error(1)
}

// --- Sheets Mock ---

type mockSheetsService struct{ mock.Mock }

func (m *mockSheetsService) RecordTerritoryTake(spreadsheetID, territoryTitle, groupTitle, publisherName string, dateTaken time.Time) error {
	args := m.Called(spreadsheetID, territoryTitle, groupTitle, publisherName, dateTaken)
	return args.Error(0)
}

func (m *mockSheetsService) RecordTerritoryReturn(spreadsheetID, territoryTitle, groupTitle, publisherName string, dateReturned time.Time) error {
	args := m.Called(spreadsheetID, territoryTitle, groupTitle, publisherName, dateReturned)
	return args.Error(0)
}

// --- Logger Mock (no-op) ---

type nopLogger struct{}

func (nopLogger) Named(string) logging.Logger                { return nopLogger{} }
func (nopLogger) With(...interface{}) logging.Logger         { return nopLogger{} }
func (nopLogger) WithContext(context.Context) logging.Logger { return nopLogger{} }
func (nopLogger) Debug(string, ...interface{})               {}
func (nopLogger) Info(string, ...interface{})                {}
func (nopLogger) Warn(string, ...interface{})                {}
func (nopLogger) Error(string, ...interface{})               {}
func (nopLogger) Fatal(string, ...interface{})               {}
func (nopLogger) Unwrap() *zapLib.Logger                     { return nil }

// --- Fake telebot.Context ---

// fakeContext captures bot responses for assertions.
type fakeContext struct {
	tb.Context // embed for interface satisfaction; only overridden methods are safe to call
	sender     *tb.User
	chat       *tb.Chat
	message    *tb.Message
	callback   *tb.Callback
	data       string
	store      map[string]interface{}
	sent       []interface{} // captures what was passed to Send()
	responded  bool
}

func newFakeContext(senderID int64, chatID int64) *fakeContext {
	return &fakeContext{
		sender: &tb.User{ID: senderID},
		chat:   &tb.Chat{ID: chatID},
		message: &tb.Message{
			Sender: &tb.User{ID: senderID},
			Chat:   &tb.Chat{ID: chatID},
		},
		store: make(map[string]interface{}),
	}
}

func (f *fakeContext) Sender() *tb.User       { return f.sender }
func (f *fakeContext) Chat() *tb.Chat         { return f.chat }
func (f *fakeContext) Message() *tb.Message   { return f.message }
func (f *fakeContext) Callback() *tb.Callback { return f.callback }
func (f *fakeContext) Data() string           { return f.data }

func (f *fakeContext) Text() string {
	if f.message != nil {
		return f.message.Text
	}
	return ""
}

func (f *fakeContext) Send(what interface{}, opts ...interface{}) error {
	f.sent = append(f.sent, what)
	return nil
}

func (f *fakeContext) Respond(resp ...*tb.CallbackResponse) error {
	f.responded = true
	return nil
}

func (f *fakeContext) Set(key string, val interface{}) {
	f.store[key] = val
}

func (f *fakeContext) Get(key string) interface{} {
	return f.store[key]
}

// --- Test Helper ---

func newTestBotService(t *testing.T) (*botService, *mockUserStorage, *mockCongregationStorage, *mockChatStorage, *mockSheetsService) {
	t.Helper()
	userStore := &mockUserStorage{}
	congStore := &mockCongregationStorage{}
	chatStore := &mockChatStorage{}
	sheetsService := &mockSheetsService{}

	svc := &botService{
		serviceContext: serviceContext{
			cfg:    &config.Config{},
			logger: nopLogger{},
			storages: Storages{
				User:         userStore,
				Congregation: congStore,
				Chat:         chatStore,
			},
			sheets: sheetsService,
		},
	}
	return svc, userStore, congStore, chatStore, sheetsService
}

// newTestBot creates a *tb.Bot backed by a mock HTTP server so b.Send/Edit/EditCaption succeed.
// The mock handles sendPhoto/sendDocument by including the corresponding media field in the response.
func newTestBot(t *testing.T) *tb.Bot {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result := map[string]interface{}{
			"message_id": 1,
			"chat":       map[string]interface{}{"id": 1},
			"date":       time.Now().Unix(),
		}
		path := r.URL.Path
		if strings.HasSuffix(path, "/sendPhoto") {
			result["photo"] = []map[string]interface{}{
				{"file_id": "photo-1", "width": 100, "height": 100},
			}
		} else if strings.HasSuffix(path, "/sendDocument") {
			result["document"] = map[string]interface{}{
				"file_id": "doc-1",
			}
		}
		resp := map[string]interface{}{
			"ok":     true,
			"result": result,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)

	bot, err := tb.NewBot(tb.Settings{
		URL:    srv.URL,
		Token:  "test:token",
		Poller: &tb.LongPoller{Timeout: 1 * time.Second},
	})
	require.NoError(t, err)
	return bot
}
