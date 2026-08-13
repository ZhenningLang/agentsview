//go:build cgo

package db

import (
	"errors"
	"fmt"

	"github.com/mattn/go-sqlite3"
)

func classifyFTSError(err error) error {
	var sqliteErr sqlite3.Error
	if errors.As(err, &sqliteErr) && sqliteErr.Code == sqlite3.ErrError {
		return &SearchInputError{
			Msg: fmt.Sprintf("search: invalid FTS query: %s", sqliteErr.Error()),
		}
	}
	return err
}

func isSQLiteUniqueConstraint(err error) bool {
	var sqliteErr sqlite3.Error
	return errors.As(err, &sqliteErr) &&
		sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique
}
