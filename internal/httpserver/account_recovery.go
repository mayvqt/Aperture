package httpserver

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/mayvqt/aperture/internal/db"
	"github.com/mayvqt/aperture/internal/mediaserver"
)

const accountCleanupTimeout = 45 * time.Second

// recoverAccount is shared by the administrator action and maintenance. The
// local gate serializes recovery, disable and deletion of each registration; the store claim also
// rejects stale actions. One Aperture process owns each state directory.
func (s *Server) recoverAccount(parent context.Context, id int64, automatic bool) (err error) {
	op, err := operationSnapshot(parent)
	if err != nil {
		return err
	}
	settings := op.Settings
	release, err := s.claimAccountOperations(id)
	if err != nil {
		return err
	}
	defer release()
	if err := parent.Err(); err != nil {
		return err
	}
	saved, err := s.store.Registration(parent, id)
	if err != nil {
		return err
	}
	if saved.BindingID != op.Identity.Binding.ID {
		return db.ErrRegistrationTransition
	}
	releaseUser, err := s.claimMediaUser(op.Identity.Binding.ID, saved.ExternalUserID.String)
	if err != nil {
		return err
	}
	defer releaseUser()
	recovery, err := s.store.ClaimTemplateRecovery(parent, id, op.Identity.Binding.ID, automatic)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), registrationProvisioningTimeout)
	defer cancel()
	reg := recovery.Registration
	if reg.UserDisableAt.Valid && !reg.UserDisableAt.Time.After(time.Now()) {
		return s.disableAccount(ctx, reg)
	}
	succeeded := false
	defer func() {
		if succeeded {
			return
		}
		persistCtx, persistCancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
		defer persistCancel()
		if recordErr := s.store.RecordTemplateRetryFailure(persistCtx, id, safeError(err)); recordErr != nil {
			slog.Error("could not record template retry failure", "registration_id", id, "error", safeError(recordErr))
		}
		s.secureIncompleteAccount(parent, id, reg.ExternalUserID.String)
		if automatic {
			description := "Aperture will retry access with backoff."
			if reg.TemplateAttempts+1 >= 6 {
				description = "Automatic access retries are finished. Review this registration before retrying access."
			}
			s.notify(webhookNotice{Event: "template.failed", Title: "Template retry failed", Description: description, Color: 0xe67e22, Fields: map[string]string{"Username": reg.Username, "Registration": strconv.FormatInt(id, 10), "Attempt": strconv.Itoa(reg.TemplateAttempts + 1), "Error": safeError(err)}})
		}
	}()
	if strings.TrimSpace(recovery.Template.PolicyJSON) == "" {
		return errors.New("saved registration template is unavailable")
	}
	if err = op.Media.ApplyTemplate(ctx, settings.ServerURL, settings.APIKey, reg.ExternalUserID.String, recovery.Template); err != nil {
		return err
	}
	if err = s.store.CompleteTemplateRecovery(ctx, id); err != nil && !s.accountCompletionConfirmed(parent, id, reg.ExternalUserID.String) {
		return err
	}
	err = nil
	succeeded = true
	s.notify(webhookNotice{Event: "template.recovered", Title: "Access template recovered", Color: 0x2ecc71, Fields: map[string]string{"Username": reg.Username, "Registration": strconv.FormatInt(id, 10), "Template": recovery.Template.Name}})
	return nil
}

func (s *Server) accountCompletionConfirmed(parent context.Context, id int64, userID string) bool {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer cancel()
	reg, err := s.store.Registration(ctx, id)
	return err == nil && reg.Status == db.RegistrationComplete && reg.ExternalUserID.Valid && reg.ExternalUserID.String == userID
}

func (s *Server) claimAccountOperations(ids ...int64) (func(), error) {
	s.accountMu.Lock()
	defer s.accountMu.Unlock()
	if s.closing {
		return nil, db.ErrRegistrationTransition
	}
	if s.activeAccounts == nil {
		s.activeAccounts = make(map[int64]bool)
	}
	for _, id := range ids {
		if s.activeAccounts[id] {
			return nil, db.ErrRegistrationTransition
		}
	}
	for _, id := range ids {
		s.activeAccounts[id] = true
	}
	s.accountWG.Add(1)
	return func() {
		s.accountMu.Lock()
		for _, id := range ids {
			delete(s.activeAccounts, id)
		}
		s.accountMu.Unlock()
		s.accountWG.Done()
	}, nil
}

func (s *Server) secureIncompleteAccount(parent context.Context, id int64, userID string) {
	op, err := operationSnapshot(parent)
	if err != nil {
		return
	}
	settings := op.Settings
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), accountCleanupTimeout)
	defer cancel()
	if err := s.store.RequireAccountCleanup(ctx, id, userID); err != nil {
		slog.Error("could not persist incomplete account cleanup", "registration_id", id, "error", safeError(err))
		if errors.Is(err, db.ErrRegistrationTransition) {
			return
		}
		if current, readErr := s.store.Registration(ctx, id); readErr == nil && current.Status == db.RegistrationComplete {
			return
		}
	}
	err = op.Media.DisableUser(ctx, settings.ServerURL, settings.APIKey, userID)
	persistCtx, persistCancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer persistCancel()
	var recordErr error
	if err != nil && !userAbsent(err) {
		recordErr = s.store.MarkUserDisableFailed(persistCtx, id, safeError(err))
		slog.Warn("incomplete account disable will retry", "registration_id", id, "error", safeError(err))
		s.notify(webhookNotice{Event: "user.disable_failed", Title: "Incomplete account disable failed", Description: "Aperture will retry disabling this account.", Color: 0xe67e22, Fields: map[string]string{"Registration": strconv.FormatInt(id, 10), "Error": safeError(err)}})
	} else {
		recordErr = s.store.MarkUserDisabled(persistCtx, id)
	}
	if recordErr != nil {
		slog.Error("could not record account cleanup result", "registration_id", id, "error", safeError(recordErr))
	}
}

func userAbsent(err error) bool {
	var httpErr *mediaserver.HTTPError
	return errors.Is(err, mediaserver.ErrUserNotFound) || (errors.As(err, &httpErr) && httpErr.StatusCode == 404)
}

func (s *Server) disableAccount(parent context.Context, reg db.Registration) error {
	op, err := operationSnapshot(parent)
	if err != nil {
		return err
	}
	if reg.BindingID != op.Identity.Binding.ID {
		return db.ErrConnectionChanged
	}
	settings := op.Settings
	ctx, cancel := context.WithTimeout(parent, accountCleanupTimeout)
	defer cancel()
	err = op.Media.DisableUser(ctx, settings.ServerURL, settings.APIKey, reg.ExternalUserID.String)
	persistCtx, persistCancel := context.WithTimeout(context.WithoutCancel(parent), 5*time.Second)
	defer persistCancel()
	if err != nil && !userAbsent(err) {
		if recordErr := s.store.MarkUserDisableFailed(persistCtx, reg.ID, safeError(err)); recordErr != nil {
			slog.Error("could not record account disable failure", "registration_id", reg.ID, "error", safeError(recordErr))
		}
		s.notify(webhookNotice{Event: "user.disable_failed", Title: "Account disable failed", Description: "Aperture will retry disabling this account.", Color: 0xe67e22, Fields: map[string]string{"Username": reg.Username, "Registration": strconv.FormatInt(reg.ID, 10), "Error": safeError(err)}})
		return err
	}
	if err := s.store.MarkUserDisabled(persistCtx, reg.ID); err != nil {
		return err
	}
	title := "Incomplete account disabled"
	if reg.UserDisableAt.Valid && !reg.UserDisableAt.Time.After(time.Now()) {
		title = "Expired user disabled"
	}
	s.notify(webhookNotice{Event: "user.disabled", Title: title, Color: 0x2ecc71, Fields: map[string]string{"Username": reg.Username, "Registration": strconv.FormatInt(reg.ID, 10)}})
	return nil
}

// The server-scoped account gate also covers separately imported/history rows
// that refer to the same upstream account.
func (s *Server) claimMediaUser(bindingID int64, userID string) (func(), error) {
	key := strconv.FormatInt(bindingID, 10) + ":" + userID
	s.accountMu.Lock()
	defer s.accountMu.Unlock()
	if s.closing || s.activeUsers[key] {
		return nil, db.ErrRegistrationTransition
	}
	if s.activeUsers == nil {
		s.activeUsers = map[string]bool{}
	}
	s.activeUsers[key] = true
	s.accountWG.Add(1)
	return func() { s.accountMu.Lock(); delete(s.activeUsers, key); s.accountMu.Unlock(); s.accountWG.Done() }, nil
}
