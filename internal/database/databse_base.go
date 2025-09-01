package database

import (
	"fmt"

	config "github.com/media-luna/eureka/configs"
	"github.com/media-luna/eureka/internal/database/mysql"
)

// Database defines the interface that all database implementations must satisfy
type Database interface {
	Setup() error
	Close() error
	InsertFingerprints(fingerprint string, songID int, offset int) error
	InsertSong(songName string, artistName string, fileHash string, totalHashes int) (int, error)
	DeleteSong(songID int) error
	UpdateSongFingerprinted(songID int) error
	Cleanup() error
	QueryFingerprints(hashes []string) ([]mysql.FingerprintMatch, error)
	GetSongByID(songID int) (mysql.SongInfo, error)
}

// NewDatabase creates a new database instance based on the configuration
func NewDatabase(cfg config.Config) (Database, error) {
	switch cfg.Database.Type {
	case "mysql":
		return mysql.NewDB(cfg)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", cfg.Database.Type)
	}
}
