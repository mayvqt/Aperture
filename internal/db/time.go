package db

import (
	"database/sql"
	"fmt"
	"time"
)

func NullableTime(value string) (sql.NullTime, error) {
	if value == "" {
		return sql.NullTime{}, nil
	}
	t, err := time.Parse("2006-01-02", value)
	if err != nil {
		return sql.NullTime{}, fmt.Errorf("date must use YYYY-MM-DD")
	}
	return sql.NullTime{Time: t.Add(24*time.Hour - time.Nanosecond).UTC(), Valid: true}, nil
}
