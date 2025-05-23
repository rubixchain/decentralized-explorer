package main

import (
	"log"
	"os"
	"path/filepath"
	"time"
)

func startWeeklySync() {
	ticker := time.NewTicker(7 * 24 * time.Hour) // 1 week
	defer ticker.Stop()

	for {
		log.Println("Weekly sync started...")

		checkTokenCount()

		if err := syncMissingCurrentOwners(); err != nil {
			log.Println("Weekly sync error:", err)
		} else {
			log.Println("Weekly sync completed successfully.")
		}

		<-ticker.C
	}
}

// Rubix Week Epoch starting reference date is Jan 01 2025.
var RubixWeekEpochStartDate = time.Date(2025, time.January, 1, 0, 0, 0, 0, time.UTC)

// To calculate the week number that's going on since reference date for a transaction
func GetWeeksPassed() int {
	now := time.Now().UTC()
	duration := now.Sub(RubixWeekEpochStartDate)
	// Handle case where current time is before ReferenceDate
	if duration < 0 {
		return 0 // If the current time is before the start date, return 0 intervals
	}
	weeksPassed := int(duration.Hours() / (24 * 7))
	// Add +1 to ensure the first week starts as week 1
	return weeksPassed + 1
}

func getAppDir() (string, error) {
	// Use executable directory
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exe), nil
}
