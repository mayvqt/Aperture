package httpserver

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/mayvqt/aperture/internal/db"
)

func inviteRows(invites []db.Invite, activity map[int64]db.InviteActivity) []inviteRow {
	rows := make([]inviteRow, 0, len(invites))
	for _, invite := range invites {
		rows = append(rows, inviteRow{
			Invite:   invite,
			Activity: activity[invite.ID],
		})
	}
	return rows
}

func defaultInviteForm() db.Invite {
	return db.Invite{MaxUses: 1}
}

func presetInviteForm(invite db.Invite) db.Invite {
	invite.ID = 0
	invite.Token = ""
	invite.TokenHash = ""
	invite.TokenPrefix = ""
	invite.Uses = 0
	invite.LastUsedAt = sql.NullTime{}
	invite.DeletedAt = sql.NullTime{}
	invite.CreatedAt = time.Time{}
	invite.UpdatedAt = time.Time{}
	return invite
}

func parseInviteExpiry(values url.Values) (sql.NullTime, error) {
	choice := strings.TrimSpace(values.Get("expires_after_days"))
	if choice == "" || choice == "custom" {
		return db.NullableTime(values.Get("expires_at"))
	}
	days, err := strconv.Atoi(choice)
	if err != nil || days < 0 {
		//lint:ignore ST1005 This validation error is rendered directly to a user.
		return sql.NullTime{}, errors.New("Choose a valid invite expiry.")
	}
	if days == 0 {
		return sql.NullTime{}, nil
	}
	if days > 3650 {
		//lint:ignore ST1005 This validation error is rendered directly to a user.
		return sql.NullTime{}, fmt.Errorf("Invite expiry must be %d days or less.", 3650)
	}
	return sql.NullTime{Time: time.Now().AddDate(0, 0, days).UTC(), Valid: true}, nil
}

func boundedFormInt(values url.Values, name string, fallback, min, max int) int {
	value, err := strconv.Atoi(strings.TrimSpace(values.Get(name)))
	if err != nil {
		return fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
