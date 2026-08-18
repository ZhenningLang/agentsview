package db

import (
	"database/sql"
	"fmt"
	"os"

	_ "github.com/mattn/go-sqlite3"
)

// CheckDataVersion verifies that an existing SQLite archive is not newer than
// this binary. Missing archives are acceptable for first daemon start.
func CheckDataVersion(path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("checking database file: %w", err)
	}
	conn, err := sql.Open("sqlite3", makeDSN(path, true))
	if err != nil {
		return fmt.Errorf("opening database for data version check: %w", err)
	}
	defer conn.Close()
	version, err := readUserVersion(conn)
	if err != nil {
		return err
	}
	if version > dataVersion {
		return fmt.Errorf("database data version %d is newer than supported version %d", version, dataVersion)
	}
	return nil
}
