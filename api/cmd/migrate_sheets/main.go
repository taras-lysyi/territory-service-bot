package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/taraslis453/territory-service-bot/config"
	"github.com/taraslis453/territory-service-bot/internal/entity"
	"github.com/taraslis453/territory-service-bot/pkg/database"
	"github.com/taraslis453/territory-service-bot/pkg/logging"
	"github.com/taraslis453/territory-service-bot/pkg/sheets"
)

// retryWithBackoff retries a function with exponential backoff on rate limit errors
func retryWithBackoff(fn func() error, maxRetries int, logger logging.Logger) error {
	var err error
	for i := 0; i < maxRetries; i++ {
		err = fn()
		if err == nil {
			return nil
		}

		// Check if it's a rate limit error
		if strings.Contains(err.Error(), "RATE_LIMIT_EXCEEDED") || strings.Contains(err.Error(), "rateLimitExceeded") {
			waitTime := time.Duration(30*(i+1)) * time.Second // 30s, 60s, 90s
			logger.Info("Rate limited, waiting before retry",
				"attempt", i+1,
				"maxRetries", maxRetries,
				"waitTime", waitTime.String(),
			)
			time.Sleep(waitTime)
			continue
		}

		// Not a rate limit error, don't retry
		return err
	}
	return err
}

// isInServiceYear checks if a date falls within the specified service year
// Service year runs from September 1 to August 31
// startYear is the year when service year starts (e.g., 2025 for 2025-2026)
func isInServiceYear(date time.Time, startYear int) bool {
	if date.IsZero() {
		return false
	}

	serviceYearStart := time.Date(startYear, time.September, 1, 0, 0, 0, 0, time.Local)
	serviceYearEnd := time.Date(startYear+1, time.August, 31, 23, 59, 59, 0, time.Local)

	return !date.Before(serviceYearStart) && !date.After(serviceYearEnd)
}

// getServiceYearStart returns the starting year of the current service year
func getServiceYearStart() int {
	now := time.Now()
	if now.Month() >= time.September {
		return now.Year()
	}
	return now.Year() - 1
}

// formatServiceYear returns the service year string (e.g., "2025-2026")
func formatServiceYear(startYear int) string {
	return fmt.Sprintf("%d-%d", startYear, startYear+1)
}

// naturalLess compares two strings naturally (so "2" < "10")
func naturalLess(a, b string) bool {
	// Extract leading numbers if present
	numA, restA := extractLeadingNumber(a)
	numB, restB := extractLeadingNumber(b)

	if numA != -1 && numB != -1 {
		if numA != numB {
			return numA < numB
		}
		// Numbers are equal, compare the rest
		return restA < restB
	}

	// Fall back to string comparison
	return a < b
}

// extractLeadingNumber extracts leading number from string
// Returns -1 if no leading number found
func extractLeadingNumber(s string) (int, string) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return -1, s
	}
	num := 0
	_, _ = fmt.Sscanf(s[:i], "%d", &num)
	return num, s[i:]
}

func main() {
	logger := logging.NewZap("setup_sheets")
	// record time exucution duration
	startTime := time.Now()
	logger.Info("Starting Google Sheets setup...", "startTime", startTime.Format(time.RFC3339))

	defer func() {
		duration := time.Since(startTime)
		logger.Info("Google Sheets setup completed", "duration", duration.String())
	}()

	// Parse command line arguments
	congregationID := flag.String("congregation", "", "Congregation ID (required)")
	force := flag.Bool("force", false, "Force recreate sheet even if it exists")
	yearFlag := flag.Int("year", 0, "Service year start (e.g., 2025 for 2025-2026). Defaults to current service year")
	skipData := flag.Bool("skip-data", false, "Only create empty sheet structure without populating territory data")
	flag.Parse()

	if *congregationID == "" {
		logger.Error("congregation ID is required")
		logger.Info("Usage: go run ./cmd/migrate_sheets -congregation=<UUID> [-force] [-year=2025] [-skip-data]")
		logger.Info("Examples:")
		logger.Info("  Current year:  go run ./cmd/migrate_sheets -congregation=<UUID>")
		logger.Info("  Future year:   go run ./cmd/migrate_sheets -congregation=<UUID> -year=2026")
		logger.Info("  Empty sheet:   go run ./cmd/migrate_sheets -congregation=<UUID> -year=2026 -skip-data")
		os.Exit(1)
	}

	// Determine service year
	serviceYearStart := *yearFlag
	if serviceYearStart == 0 {
		serviceYearStart = getServiceYearStart()
	}
	serviceYearName := formatServiceYear(serviceYearStart)

	logger.Info("Setting up Google Sheet for congregation",
		"congregationID", *congregationID,
		"serviceYear", serviceYearName,
	)

	cfg := config.Get()

	// Initialize database
	sql, err := database.NewPostgreSQL(&database.PostgreSQLConfig{
		User:     cfg.User,
		Password: cfg.Password,
		Host:     cfg.Host,
		Database: cfg.Database,
	})
	if err != nil {
		logger.Fatal("failed to init postgresql", "err", err)
	}

	// Get congregation
	var congregation entity.Congregation
	err = sql.DB.Where("id = ?", *congregationID).First(&congregation).Error
	if err != nil {
		logger.Fatal("failed to get congregation", "err", err, "congregationID", *congregationID)
	}

	logger.Info("Found congregation", "name", congregation.Name)

	// Check if spreadsheet ID is set
	if congregation.SpreadsheetID == "" {
		logger.Fatal("congregation has no SpreadsheetID set. Please set it first in the database.")
	}

	logger.Info("Using spreadsheet", "spreadsheetID", congregation.SpreadsheetID)

	// Initialize Google Sheets service
	sheetsService, err := sheets.New(cfg.CredentialsJSON)
	if err != nil {
		logger.Fatal("failed to init sheets service", "err", err)
	}

	// Check if sheet already exists
	sheetName := serviceYearName
	exists, err := sheetsService.SheetExists(congregation.SpreadsheetID, sheetName)
	if err != nil {
		logger.Fatal("failed to check if sheet exists", "err", err)
	}

	if exists && !*force {
		logger.Error("Sheet already exists! Use -force flag to recreate it.",
			"sheetName", sheetName,
			"spreadsheetLink", sheets.GetSpreadsheetLink(congregation.SpreadsheetID),
		)
		os.Exit(1)
	}

	if exists && *force {
		logger.Info("Sheet exists, will recreate it (force mode)", "sheetName", sheetName)
	}

	// Get all territory groups for this congregation
	var groups []entity.CongregationTerritoryGroup
	err = sql.DB.Where("congregation_id = ?", congregation.ID).Find(&groups).Error
	if err != nil {
		logger.Fatal("failed to get territory groups", "err", err)
	}

	// Create a map of group ID to group title
	groupMap := make(map[string]string)
	for _, group := range groups {
		groupMap[group.ID] = group.Title
	}

	// Get all territories for this congregation
	var territories []entity.CongregationTerritory
	err = sql.DB.Where("congregation_id = ?", congregation.ID).Find(&territories).Error
	if err != nil {
		logger.Fatal("failed to get territories", "err", err)
	}

	logger.Info("Found territories", "count", len(territories))

	// Sort territories by group title, then by territory title
	sort.Slice(territories, func(i, j int) bool {
		groupI := groupMap[territories[i].GroupID]
		groupJ := groupMap[territories[j].GroupID]
		if groupI != groupJ {
			return groupI < groupJ
		}
		return naturalLess(territories[i].Title, territories[j].Title)
	})

	// Build TerritorySetup list (already sorted)
	var territorySetups []sheets.TerritorySetup
	for _, territory := range territories {
		groupTitle := groupMap[territory.GroupID]
		territorySetups = append(territorySetups, sheets.TerritorySetup{
			Number: territory.Title,
			Place:  groupTitle,
		})
	}

	// Setup the sheet with all territories
	// Use September 1 of the target service year as the date
	serviceYearDate := time.Date(serviceYearStart, time.September, 1, 0, 0, 0, 0, time.Local)
	logger.Info("Creating sheet with territories...", "sheetName", sheetName)
	err = sheetsService.SetupTerritories(congregation.SpreadsheetID, territorySetups, serviceYearDate)
	if err != nil {
		logger.Fatal("failed to setup territories", "err", err)
	}

	// If skip-data flag is set, we're done
	if *skipData {
		logger.Info("Sheet created (skip-data mode - no territory data populated)",
			"sheetName", sheetName,
			"totalTerritories", len(territories),
			"spreadsheetLink", sheets.GetSpreadsheetLink(congregation.SpreadsheetID),
		)
		return
	}

	logger.Info("Sheet created, now populating data...")

	// Get all users for this congregation (to get publisher names)
	var users []entity.User
	err = sql.DB.Where("congregation_id = ?", congregation.ID).Find(&users).Error
	if err != nil {
		logger.Fatal("failed to get users", "err", err)
	}

	// Create a map of user ID to user name
	userMap := make(map[string]string)
	for _, user := range users {
		userMap[user.ID] = user.FullName
	}

	// Process each territory - only those with dates in target service year
	inUseCount := 0
	notInUseCount := 0
	skippedCount := 0
	errorCount := 0

	for i, territory := range territories {
		groupTitle := groupMap[territory.GroupID]

		// Skip territories with dates outside target service year
		if !isInServiceYear(territory.LastTakenAt, serviceYearStart) {
			skippedCount++
			logger.Info("Skipping territory (date outside service year)",
				"progress", fmt.Sprintf("%d/%d", i+1, len(territories)),
				"territory", territory.Title,
				"lastTakenAt", territory.LastTakenAt.Format("02.01.2006"),
				"serviceYear", serviceYearName,
			)
			continue
		}

		logger.Info("Processing territory",
			"progress", fmt.Sprintf("%d/%d", i+1, len(territories)),
			"territory", territory.Title,
			"group", groupTitle,
		)

		if territory.InUseByUserID != nil && *territory.InUseByUserID != "" {
			// Territory is in use - add take record without return date
			publisherName := userMap[*territory.InUseByUserID]
			if publisherName == "" {
				publisherName = "Unknown"
			}

			err = retryWithBackoff(func() error {
				return sheetsService.RecordTerritoryTakeWithoutReturn(
					congregation.SpreadsheetID,
					territory.Title,
					groupTitle,
					publisherName,
					territory.LastTakenAt,
				)
			}, 3, logger)

			if err != nil {
				logger.Error("failed to record territory take",
					"err", err,
					"territory", territory.Title,
					"publisher", publisherName,
				)
				errorCount++
			} else {
				inUseCount++
				logger.Info("Recorded territory take",
					"territory", territory.Title,
					"publisher", publisherName,
					"dateTaken", territory.LastTakenAt.Format("02.01.2006"),
				)
			}
		} else {
			// Territory is not in use - just update last processed date
			err = retryWithBackoff(func() error {
				return sheetsService.UpdateLastProcessedDate(
					congregation.SpreadsheetID,
					territory.Title,
					groupTitle,
					territory.LastTakenAt,
				)
			}, 3, logger)

			if err != nil {
				logger.Error("failed to update last processed date",
					"err", err,
					"territory", territory.Title,
					"group", groupTitle,
				)
				errorCount++
			} else {
				notInUseCount++
				logger.Info("Updated last processed date",
					"territory", territory.Title,
					"date", territory.LastTakenAt.Format("02.01.2006"),
				)
			}
		}

		// Delay to avoid rate limiting (60 reads/min = 1 read/sec, we do ~3 reads per territory)
		time.Sleep(3 * time.Second)
	}

	logger.Info("Setup completed!",
		"congregation", congregation.Name,
		"totalTerritories", len(territories),
		"territoriesInUse", inUseCount,
		"territoriesNotInUse", notInUseCount,
		"skippedOldDates", skippedCount,
		"errors", errorCount,
		"spreadsheetLink", sheets.GetSpreadsheetLink(congregation.SpreadsheetID),
	)
}
