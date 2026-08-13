//go:build !cgo

package db

import "errors"

var ErrSQLiteRequiresCGO = errors.New("sqlite support requires cgo")

func classifyFTSError(err error) error {
	// Without cgo the SQLite driver cannot open a database, so no FTS-specific
	// SQLite error code is available to classify.
	return err
}

func isSQLiteUniqueConstraint(_ error) bool {
	return false
}
