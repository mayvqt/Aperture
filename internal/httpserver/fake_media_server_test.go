package httpserver

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/mayvqt/aperture/internal/db"
	"github.com/mayvqt/aperture/internal/mediaserver"
)

type fakeMediaServer struct {
	createdUser     bool
	appliedTemplate bool
	appliedUserID   string
	appliedPolicy   string
	disabledUserID  string
	adminRevoked    bool
	isAdminErr      error
	authErr         error
	createErr       error
	applyErr        error
	importErr       error
	pingErr         error
	pingCalls       atomic.Int32
	pingStarted     chan struct{}
	pingRelease     chan struct{}
	pingStartOnce   sync.Once
	pingAPIKey      atomic.Value
	provider        mediaserver.Provider
	userExists      bool
	userExistsErr   error
	users           []mediaserver.User
	deletedUserID   string
}

func (f *fakeMediaServer) SetProvider(provider mediaserver.Provider) error {
	f.provider = provider
	return nil
}

func (f *fakeMediaServer) Authenticate(context.Context, string, string, string) (mediaserver.AuthResult, error) {
	if f.authErr != nil {
		return mediaserver.AuthResult{}, f.authErr
	}
	return mediaserver.AuthResult{UserID: "admin-id", Username: "admin", AccessToken: "access-token", DeviceID: "new-device-id", IsAdmin: true}, nil
}
func (f *fakeMediaServer) IsAdmin(context.Context, string, string, string, string) (bool, error) {
	return !f.adminRevoked, f.isAdminErr
}
func (f *fakeMediaServer) Ping(_ context.Context, _, apiKey string) error {
	f.pingCalls.Add(1)
	f.pingAPIKey.Store(apiKey)
	if f.pingStarted != nil {
		f.pingStartOnce.Do(func() { close(f.pingStarted) })
	}
	if f.pingRelease != nil {
		<-f.pingRelease
	}
	if f.pingErr != nil {
		return f.pingErr
	}
	return nil
}
func (f *fakeMediaServer) CreateUser(context.Context, string, string, string, string) (mediaserver.User, error) {
	f.createdUser = true
	if f.createErr != nil {
		return mediaserver.User{}, f.createErr
	}
	return mediaserver.User{ID: "new-media-user", Name: "new_user"}, nil
}
func (f *fakeMediaServer) ApplyTemplate(_ context.Context, _, _, userID string, template db.Template) error {
	f.appliedTemplate = true
	f.appliedUserID = userID
	f.appliedPolicy = template.PolicyJSON
	return f.applyErr
}
func (f *fakeMediaServer) DisableUser(_ context.Context, _ string, _ string, userID string) error {
	f.disabledUserID = userID
	return nil
}
func (f *fakeMediaServer) UserExists(context.Context, string, string, string) (bool, error) {
	return f.userExists, f.userExistsErr
}
func (f *fakeMediaServer) ListUsers(context.Context, string, string) ([]mediaserver.User, error) {
	return f.users, nil
}
func (f *fakeMediaServer) DeleteUser(_ context.Context, _, _, userID string) error {
	f.deletedUserID = userID
	return nil
}
func (f *fakeMediaServer) ImportTemplate(context.Context, string, string, string, string) (mediaserver.TemplateData, error) {
	if f.importErr != nil {
		return mediaserver.TemplateData{}, f.importErr
	}
	return mediaserver.TemplateData{PolicyJSON: db.TemplatePolicyDefaultJSON}, nil
}

var _ MediaServer = (*fakeMediaServer)(nil)

var errFakePing = errors.New("ping failed")
