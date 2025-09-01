package mysql

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	config "github.com/media-luna/eureka/configs"
	"github.com/media-luna/eureka/utils/logger"
)

// DB represents a MySQL database connection.
type DB struct {
	conn *sql.DB
	cfg  config.Config
}

// Song represents a song record from the database
type Song struct {
	ID            int
	Name          string
	Artist        string
	Fingerprinted bool
	FileSHA1      string
	TotalHashes   int
	DateCreated   string
}

// FingerprintMatch represents a fingerprint match from the database
type FingerprintMatch struct {
	Hash   string
	SongID int
	Offset int
}

// SongInfo represents basic song information
type SongInfo struct {
	ID     int
	Name   string
	Artist string
}

const (
	createSongsTableSQL = `
		CREATE TABLE IF NOT EXISTS %s (
			%s MEDIUMINT UNSIGNED NOT NULL AUTO_INCREMENT,
			%s VARCHAR(250) NOT NULL,
			%s VARCHAR(250) DEFAULT '',
			%s TINYINT DEFAULT 0,
			%s BINARY(20) NOT NULL,
			%s INT NOT NULL DEFAULT 0,
			date_created DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			date_modified DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			PRIMARY KEY (%s),
			UNIQUE KEY file_sha1_idx (%s)
		) ENGINE=INNODB;`

	createFingerprintsTableSQL = `
		CREATE TABLE IF NOT EXISTS %s (
			%s CHAR(40) NOT NULL,
			%s MEDIUMINT UNSIGNED NOT NULL,
			%s INT UNSIGNED NOT NULL,
			date_created DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
			date_modified DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			INDEX ix_%s_%s (%s),
			CONSTRAINT uq_%s_%s_%s_%s UNIQUE KEY (%s, %s, %s),
			CONSTRAINT fk_%s_%s FOREIGN KEY (%s)
				REFERENCES %s(%s) ON DELETE CASCADE
		) ENGINE=INNODB;`

	deleteUnfingerprintedSQL = `DELETE FROM %s WHERE %s = 0;`
)

// NewDB creates a new DB instance with the given configuration.
func NewDB(cfg config.Config) (*DB, error) {
	db := &DB{cfg: cfg}
	if err := db.connect(); err != nil {
		return nil, err
	}
	return db, nil
}

// Connect to the MySQL database.
func (m *DB) connect() error {
	var err error
	dsnString := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?%s",
		m.cfg.Database.User,
		m.cfg.Database.Password,
		m.cfg.Database.Host,
		m.cfg.Database.Port,
		m.cfg.Database.DBName,
		m.cfg.Database.Params)

	m.conn, err = sql.Open("mysql", dsnString)
	if err != nil {
		return fmt.Errorf("failed to open connection: %w", err)
	}

	// Test the connection
	if err := m.conn.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}
	logger.Info("Connected to MySQL database")
	return nil
}

// Setup initializes the database tables.
func (m *DB) Setup() error {
	// Create songs table
	songsSQL := fmt.Sprintf(createSongsTableSQL,
		m.cfg.Tables.Songs.Name,
		m.cfg.Tables.Songs.Fields.ID,
		m.cfg.Tables.Songs.Fields.Name,
		m.cfg.Tables.Songs.Fields.Artist,
		m.cfg.Tables.Songs.Fields.Fingerprinted,
		m.cfg.Tables.Songs.Fields.FileSHA1,
		m.cfg.Tables.Songs.Fields.TotalHashes,
		m.cfg.Tables.Songs.Fields.ID,
		m.cfg.Tables.Songs.Fields.FileSHA1)

	if _, err := m.conn.Exec(songsSQL); err != nil {
		return fmt.Errorf("error creating songs table: %w", err)
	}

	// Create fingerprints table
	fpSQL := fmt.Sprintf(createFingerprintsTableSQL,
		m.cfg.Tables.Fingerprints.Name,
		m.cfg.Tables.Fingerprints.Fields.Hash,
		m.cfg.Tables.Songs.Fields.ID,
		m.cfg.Tables.Fingerprints.Fields.Offset,
		m.cfg.Tables.Fingerprints.Name,
		m.cfg.Tables.Fingerprints.Fields.Hash,
		m.cfg.Tables.Fingerprints.Fields.Hash,
		m.cfg.Tables.Fingerprints.Name,
		m.cfg.Tables.Songs.Fields.ID,
		m.cfg.Tables.Fingerprints.Fields.Offset,
		m.cfg.Tables.Fingerprints.Fields.Hash,
		m.cfg.Tables.Songs.Fields.ID,
		m.cfg.Tables.Fingerprints.Fields.Offset,
		m.cfg.Tables.Fingerprints.Fields.Hash,
		m.cfg.Tables.Fingerprints.Name,
		m.cfg.Tables.Songs.Fields.ID,
		m.cfg.Tables.Songs.Fields.ID,
		m.cfg.Tables.Songs.Name,
		m.cfg.Tables.Songs.Fields.ID)

	if _, err := m.conn.Exec(fpSQL); err != nil {
		return fmt.Errorf("error creating fingerprints table: %w", err)
	}

	return nil
}

// Close the MySQL database connection.
func (m *DB) Close() error {
	return m.conn.Close()
}

// Insert song metadata into songs table
func (m *DB) insertSongWithID(songName string, artistName string, fileHash string, totalHashes int) (int64, error) {
	// Check if song with same hash already exists
	var existingID int64
	query := fmt.Sprintf("SELECT %s FROM %s WHERE HEX(%s) = ?",
		m.cfg.Tables.Songs.Fields.ID,
		m.cfg.Tables.Songs.Name,
		m.cfg.Tables.Songs.Fields.FileSHA1)

	err := m.conn.QueryRow(query, fileHash).Scan(&existingID)
	if err != sql.ErrNoRows {
		if err == nil {
			// Verify that the song still exists by ID
			verifyQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = ?",
				m.cfg.Tables.Songs.Name,
				m.cfg.Tables.Songs.Fields.ID)

			var count int
			verifyErr := m.conn.QueryRow(verifyQuery, existingID).Scan(&count)
			if verifyErr != nil {
				return 0, fmt.Errorf("error verifying existing song: %w", verifyErr)
			}

			if count > 0 {
				logger.Info(fmt.Sprintf("Found existing song: %s", songName))
				return existingID, nil
			}
			// The song entry no longer exists despite the hash match
			logger.Info(fmt.Sprintf("Found hash for song %s, but the record doesn't exist - will create new entry", songName))
		} else {
			return 0, fmt.Errorf("error checking for existing song: %w", err)
		}
	}

	// Insert new song if it doesn't exist
	insertQuery := fmt.Sprintf("INSERT INTO %s (%s, %s, %s, %s, %s) VALUES (?, ?, UNHEX(?), ?, ?)",
		m.cfg.Tables.Songs.Name,
		m.cfg.Tables.Songs.Fields.Name,
		m.cfg.Tables.Songs.Fields.Artist,
		m.cfg.Tables.Songs.Fields.FileSHA1,
		m.cfg.Tables.Songs.Fields.TotalHashes,
		m.cfg.Tables.Songs.Fields.Fingerprinted)

	result, err := m.conn.Exec(insertQuery, songName, artistName, fileHash, totalHashes, 0)
	if err != nil {
		return 0, fmt.Errorf("error inserting song: %w", err)
	}

	id, err := result.LastInsertId()
	if err == nil {
		logger.Info(fmt.Sprintf("Added new song: %s", songName))
	}
	return id, err
}

// InsertSong implements the Database interface
func (m *DB) InsertSong(songName string, artistName string, fileHash string, totalHashes int) (int, error) {
	id, err := m.insertSongWithID(songName, artistName, fileHash, totalHashes)
	return int(id), err
}

// Insert fingerprints into fingerprints table
func (m *DB) InsertFingerprints(fingerprint string, songID int, offset int) error {
	query := fmt.Sprintf("INSERT IGNORE INTO %s (%s, %s, %s) VALUES (?, ?, ?)",
		m.cfg.Tables.Fingerprints.Name,
		m.cfg.Tables.Songs.Fields.ID,
		m.cfg.Tables.Fingerprints.Fields.Hash,
		m.cfg.Tables.Fingerprints.Fields.Offset)

	_, err := m.conn.Exec(query, songID, fingerprint, offset)
	return err
}

// UpdateSongFingerprinted marks a song as fingerprinted in the database
func (m *DB) UpdateSongFingerprinted(songID int) error {
	// First check if the song exists
	checkQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s = ?",
		m.cfg.Tables.Songs.Name,
		m.cfg.Tables.Songs.Fields.ID)

	var count int
	err := m.conn.QueryRow(checkQuery, songID).Scan(&count)
	if err != nil {
		return fmt.Errorf("error checking if song exists: %w", err)
	}

	if count == 0 {
		return fmt.Errorf("song with ID %d not found", songID)
	}

	// Song exists, update it
	updateQuery := fmt.Sprintf("UPDATE %s SET %s = 1 WHERE %s = ?",
		m.cfg.Tables.Songs.Name,
		m.cfg.Tables.Songs.Fields.Fingerprinted,
		m.cfg.Tables.Songs.Fields.ID)

	_, err = m.conn.Exec(updateQuery, songID)
	if err != nil {
		return fmt.Errorf("error updating song fingerprinted status: %w", err)
	}

	return nil
}

// ListSongs returns all songs from the database
func (m *DB) ListSongs() ([]Song, error) {
	query := fmt.Sprintf("SELECT %s, %s, artist, %s, HEX(%s), %s, date_created FROM %s",
		m.cfg.Tables.Songs.Fields.ID,
		m.cfg.Tables.Songs.Fields.Name,
		m.cfg.Tables.Songs.Fields.Fingerprinted,
		m.cfg.Tables.Songs.Fields.FileSHA1,
		m.cfg.Tables.Songs.Fields.TotalHashes,
		m.cfg.Tables.Songs.Name)

	rows, err := m.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("error querying songs: %w", err)
	}
	defer rows.Close()

	var songs []Song
	for rows.Next() {
		var s Song
		if err := rows.Scan(&s.ID, &s.Name, &s.Artist, &s.Fingerprinted, &s.FileSHA1, &s.TotalHashes, &s.DateCreated); err != nil {
			return nil, fmt.Errorf("error scanning song row: %w", err)
		}
		songs = append(songs, s)
	}

	return songs, nil
}

// Cleanup performs general database cleanup:
// 1. Removes duplicate songs keeping only the fingerprinted ones
// 2. Removes unfingerprinted songs
// 3. Removes orphaned fingerprints (those without corresponding songs)
func (m *DB) Cleanup() error {
	// Keep only fingerprinted songs if duplicates exist
	duplicatesQuery := fmt.Sprintf(`
		DELETE s1 FROM %s s1
		INNER JOIN %s s2
		WHERE s1.%s = s2.%s
		AND s1.%s = 0
		AND s2.%s = 1`,
		m.cfg.Tables.Songs.Name,
		m.cfg.Tables.Songs.Name,
		m.cfg.Tables.Songs.Fields.FileSHA1,
		m.cfg.Tables.Songs.Fields.FileSHA1,
		m.cfg.Tables.Songs.Fields.Fingerprinted,
		m.cfg.Tables.Songs.Fields.Fingerprinted)

	result, err := m.conn.Exec(duplicatesQuery)
	if err != nil {
		return fmt.Errorf("error cleaning up duplicates: %w", err)
	}

	if rows, _ := result.RowsAffected(); rows > 0 {
		logger.Info(fmt.Sprintf("Cleaned up %d duplicate songs", rows))
	}

	// Delete unfingerprinted songs
	unfingerSQL := fmt.Sprintf(deleteUnfingerprintedSQL,
		m.cfg.Tables.Songs.Name,
		m.cfg.Tables.Songs.Fields.Fingerprinted)

	result, err = m.conn.Exec(unfingerSQL)
	if err != nil {
		return fmt.Errorf("error cleaning up unfingerprinted songs: %w", err)
	}

	if rows, _ := result.RowsAffected(); rows > 0 {
		logger.Info(fmt.Sprintf("Cleaned up %d unfingerprinted songs", rows))
	}

	// Delete orphaned fingerprints (those without corresponding songs)
	orphanedFPQuery := fmt.Sprintf(`
		DELETE fp FROM %s fp
		LEFT JOIN %s s ON fp.%s = s.%s
		WHERE s.%s IS NULL`,
		m.cfg.Tables.Fingerprints.Name,
		m.cfg.Tables.Songs.Name,
		m.cfg.Tables.Songs.Fields.ID,
		m.cfg.Tables.Songs.Fields.ID,
		m.cfg.Tables.Songs.Fields.ID)

	result, err = m.conn.Exec(orphanedFPQuery)
	if err != nil {
		return fmt.Errorf("error cleaning up orphaned fingerprints: %w", err)
	}

	if rows, _ := result.RowsAffected(); rows > 0 {
		logger.Info(fmt.Sprintf("Cleaned up %d orphaned fingerprints", rows))
	}

	return nil
}

// DeleteSong deletes a song and its fingerprints from the database
func (m *DB) DeleteSong(songID int) error {
	// Since we have ON DELETE CASCADE, we only need to delete the song
	// and the fingerprints will be automatically deleted
	query := fmt.Sprintf("DELETE FROM %s WHERE %s = ?",
		m.cfg.Tables.Songs.Name,
		m.cfg.Tables.Songs.Fields.ID)

	result, err := m.conn.Exec(query, songID)
	if err != nil {
		return fmt.Errorf("error deleting song: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error getting affected rows: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("song with ID %d not found", songID)
	}

	logger.Info(fmt.Sprintf("Successfully deleted song with ID %d", songID))
	return nil
}

// QueryFingerprints queries the database for matching fingerprints
func (m *DB) QueryFingerprints(hashes []string) ([]FingerprintMatch, error) {
	if len(hashes) == 0 {
		return []FingerprintMatch{}, nil
	}

	// Build query with placeholders
	placeholders := strings.Repeat("?,", len(hashes))
	placeholders = placeholders[:len(placeholders)-1] // Remove last comma

	query := fmt.Sprintf(`
		SELECT %s, %s, %s 
		FROM %s 
		WHERE %s IN (%s)`,
		m.cfg.Tables.Fingerprints.Fields.Hash,
		m.cfg.Tables.Songs.Fields.ID,
		m.cfg.Tables.Fingerprints.Fields.Offset,
		m.cfg.Tables.Fingerprints.Name,
		m.cfg.Tables.Fingerprints.Fields.Hash,
		placeholders)

	// Convert hashes to interface{} slice for query
	args := make([]interface{}, len(hashes))
	for i, hash := range hashes {
		args[i] = hash
	}

	rows, err := m.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("error querying fingerprints: %w", err)
	}
	defer rows.Close()

	var matches []FingerprintMatch
	for rows.Next() {
		var match FingerprintMatch
		if err := rows.Scan(&match.Hash, &match.SongID, &match.Offset); err != nil {
			return nil, fmt.Errorf("error scanning fingerprint match: %w", err)
		}
		matches = append(matches, match)
	}

	return matches, nil
}

// GetSongByID retrieves song information by ID
func (m *DB) GetSongByID(songID int) (SongInfo, error) {
	query := fmt.Sprintf("SELECT %s, %s, %s FROM %s WHERE %s = ?",
		m.cfg.Tables.Songs.Fields.ID,
		m.cfg.Tables.Songs.Fields.Name,
		m.cfg.Tables.Songs.Fields.Artist,
		m.cfg.Tables.Songs.Name,
		m.cfg.Tables.Songs.Fields.ID)

	var song SongInfo
	err := m.conn.QueryRow(query, songID).Scan(&song.ID, &song.Name, &song.Artist)
	if err != nil {
		if err == sql.ErrNoRows {
			return SongInfo{}, fmt.Errorf("song with ID %d not found", songID)
		}
		return SongInfo{}, fmt.Errorf("error querying song: %w", err)
	}

	return song, nil
}
