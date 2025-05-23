package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	config "github.com/media-luna/eureka/configs"
	"github.com/media-luna/eureka/internal/eureka"
	"github.com/media-luna/eureka/utils/logger"
)

func main() {
	// Parse command line arguments
	audioFile := flag.String("file", "", "Path to the audio file to process")
	listCmd := flag.Bool("list", false, "List all songs in the database")
	cleanupCmd := flag.Bool("cleanup", false, "Clean up duplicate songs in the database")
	deleteCmd := flag.Int("delete", -1, "Delete a song by its ID")
	flag.Parse()

	// Load configuration
	dir, _ := os.Getwd()
	configFilePath := filepath.Join(dir, "configs", "config.yaml")
	config, err := config.LoadConfig(configFilePath)
	if err != nil {
		logger.Error(fmt.Errorf("failed to load configuration: %v", err))
		os.Exit(1)
	}

	// Get Eureka app
	app, err := eureka.NewEureka(*config)
	if err != nil {
		logger.Error(fmt.Errorf("error initializing Eureka: %v", err))
		os.Exit(1)
	}

	if *deleteCmd >= 0 {
		if err := app.Delete(*deleteCmd); err != nil {
			logger.Error(fmt.Errorf("error deleting song: %v", err))
			os.Exit(1)
		}
		return
	}

	if *cleanupCmd {
		if err := app.Cleanup(); err != nil {
			logger.Error(fmt.Errorf("error cleaning up duplicates: %v", err))
			os.Exit(1)
		}
		return
	}

	if *listCmd {
		songs, err := app.List()
		if err != nil {
			logger.Error(fmt.Errorf("error listing songs: %v", err))
			os.Exit(1)
		}
		if len(songs) == 0 {
			logger.Info("No songs found in the database")
			return
		}
		logger.Info("Found songs in database:")
		for _, song := range songs {
			fmt.Printf("ID: %d | Name: %s | Artist: %s | Fingerprinted: %v | Hashes: %d | Created: %s\n",
				song.ID, song.Name, song.Artist, song.Fingerprinted, song.TotalHashes, song.DateCreated)
		}
		return
	}

	if *audioFile == "" {
		logger.Error(fmt.Errorf("please provide an audio file path using -file flag or use -list to see database contents"))
		flag.Usage()
		os.Exit(1)
	}

	if err := app.Save(*audioFile); err != nil {
		logger.Error(fmt.Errorf("failed to process audio file: %v", err))
		os.Exit(1)
	}
}
