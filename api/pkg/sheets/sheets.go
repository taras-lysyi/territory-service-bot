package sheets

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/option"
	"google.golang.org/api/sheets/v4"
)

// Service provides Google Sheets operations for territory tracking
type Service struct {
	client *sheets.Service
}

// Constants for sheet structure
const (
	// Row indices (0-based for API)
	titleRow       = 0 // "ЗАПИСИ ПРО ОПРАЦЮВАННЯ ТЕРИТОРІЙ"
	serviceYearRow = 2 // "Службовий рік: XXXX - XXXX"
	separatorRow   = 3 // Empty row with bottom border
	headerRow1     = 4 // Header row 1 (№ Тер., Місце, etc.)
	headerRow2     = 5 // Header row 2 (Дата отримання, Дата опрацювання)
	dataStartRow   = 6 // First territory data row (0-based)

	// Column indices (0-based)
	colTerritoryNum = 0 // A - № Тер.
	colPlace        = 1 // B - Місце
	colLastDate     = 2 // C - Остання дата опрацювання
	colFirstVisnik  = 3 // D - First publisher column

	// Each publisher entry uses 2 columns (name merged on row1, dates on row2)
	colsPerPublisher = 2

	// Column widths in pixels
	colWidthTerritoryNum = 60   // № Тер.
	colWidthPlace        = 160  // Місце (wider for "Військова частина")
	colWidthLastDate     = 140  // Остання дата опрацювання
	colWidthPublisher    = 130  // Publisher name / dates

	// Formatting colors
	headerBgColor   = 0xD9D9D9 // Gray background for headers
	headerTextColor = 0x404040 // Dark gray text for headers
	greenTextColor  = 0x38761D // Green for completed
	redTextColor    = 0xFF0000 // Red for overdue
)

// New creates a new Google Sheets service
func New(credentialsJSON string) (*Service, error) {
	ctx := context.Background()

	// Try to decode base64 first, if it fails assume it's raw JSON
	creds, err := base64.StdEncoding.DecodeString(credentialsJSON)
	if err != nil {
		creds = []byte(credentialsJSON)
	}

	config, err := google.JWTConfigFromJSON(creds, sheets.SpreadsheetsScope)
	if err != nil {
		return nil, fmt.Errorf("failed to parse credentials: %w", err)
	}

	client := config.Client(ctx)
	srv, err := sheets.NewService(ctx, option.WithHTTPClient(client))
	if err != nil {
		return nil, fmt.Errorf("failed to create sheets service: %w", err)
	}

	return &Service{client: srv}, nil
}

// GetServiceYear returns the service year name for a given date
// Service year runs from September to August (e.g., "2024-2025" for Sep 2024 - Aug 2025)
func GetServiceYear(t time.Time) string {
	year := t.Year()
	month := t.Month()

	// If we're in September or later, the service year starts this year
	// If we're before September, the service year started last year
	if month >= time.September {
		return fmt.Sprintf("%d-%d", year, year+1)
	}
	return fmt.Sprintf("%d-%d", year-1, year)
}

// hexToColor converts a hex color to sheets.Color
func hexToColor(hex int64) *sheets.Color {
	return &sheets.Color{
		Red:   float64((hex>>16)&0xFF) / 255.0,
		Green: float64((hex>>8)&0xFF) / 255.0,
		Blue:  float64(hex&0xFF) / 255.0,
	}
}

// getSheetID gets the sheet ID by name
func (s *Service) getSheetID(ctx context.Context, spreadsheetID, sheetName string) (int64, error) {
	spreadsheet, err := s.client.Spreadsheets.Get(spreadsheetID).Context(ctx).Do()
	if err != nil {
		return 0, fmt.Errorf("failed to get spreadsheet: %w", err)
	}

	for _, sheet := range spreadsheet.Sheets {
		if sheet.Properties.Title == sheetName {
			return sheet.Properties.SheetId, nil
		}
	}

	return 0, fmt.Errorf("sheet %s not found", sheetName)
}

// RecordTerritoryTake records when a territory is taken by a publisher
// In the new format: publisher name on row1 (merged across 2 cols), date taken on row2 col1
func (s *Service) RecordTerritoryTake(spreadsheetID, territoryTitle, groupTitle, publisherName string, dateTaken time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	sheetName := GetServiceYear(dateTaken)

	// Check if the sheet exists - do NOT create it automatically
	// The sheet should be initialized with all territories beforehand
	sheetExists, err := s.SheetExists(spreadsheetID, sheetName)
	if err != nil {
		return fmt.Errorf("failed to check if sheet exists: %w", err)
	}
	if !sheetExists {
		return fmt.Errorf("sheet %s does not exist in spreadsheet - please initialize the sheet with all territories first", sheetName)
	}

	// Find the territory row by both title AND group (composite key)
	rowIndex, err := s.findTerritoryRow(ctx, spreadsheetID, sheetName, territoryTitle, groupTitle)
	if err != nil {
		return fmt.Errorf("failed to find territory row: %w", err)
	}

	// If territory not found, return error - don't add single territory rows
	if rowIndex == -1 {
		return fmt.Errorf("territory %s in group %s not found in sheet %s - please add it to the sheet first", territoryTitle, groupTitle, sheetName)
	}

	// Find the next available column for this territory
	nextCol, err := s.findNextAvailableColumn(ctx, spreadsheetID, sheetName, rowIndex)
	if err != nil {
		return fmt.Errorf("failed to find next available column: %w", err)
	}

	// Write publisher name on row1 (will be merged) and date taken on row2
	dateStr := dateTaken.Format("02.01.2006")

	// Row 1: Publisher name (spans 2 columns when merged)
	// Row 2: Date taken (first column), Date returned will go in second column
	writeRange := fmt.Sprintf("'%s'!%s%d:%s%d", sheetName, columnToLetter(nextCol), rowIndex, columnToLetter(nextCol+1), rowIndex+1)

	_, err = s.client.Spreadsheets.Values.Update(spreadsheetID, writeRange, &sheets.ValueRange{
		Values: [][]interface{}{
			{publisherName, ""},       // Row 1: publisher name (merged across 2 cols)
			{dateStr, ""},             // Row 2: date taken, date returned (empty)
		},
	}).ValueInputOption("USER_ENTERED").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to write take data: %w", err)
	}

	// Apply formatting - merge publisher name cells, add borders, add header if first for this column
	sheetID, err := s.getSheetID(ctx, spreadsheetID, sheetName)
	if err != nil {
		return nil // Data written, formatting is optional
	}

	// Add Вісник header for this column if it's a new column
	err = s.addPublisherHeader(ctx, spreadsheetID, sheetName, sheetID, nextCol)
	if err != nil {
		// Log but don't fail
	}

	err = s.formatPublisherEntry(ctx, spreadsheetID, sheetID, rowIndex, nextCol)
	if err != nil {
		// Log but don't fail - data is already written
		return nil
	}

	return nil
}

// addPublisherHeader adds the Вісник header for a new publisher column
func (s *Service) addPublisherHeader(ctx context.Context, spreadsheetID, sheetName string, sheetID int64, colIndex int) error {
	// Check if header already exists
	headerRange := fmt.Sprintf("'%s'!%s5", sheetName, columnToLetter(colIndex))
	resp, err := s.client.Spreadsheets.Values.Get(spreadsheetID, headerRange).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to check header: %w", err)
	}

	// If header exists, skip
	if len(resp.Values) > 0 && len(resp.Values[0]) > 0 && resp.Values[0][0] != nil && resp.Values[0][0] != "" {
		return nil
	}

	// Write header: Row 5 = "Вісник", Row 6 = "Дата отримання" | "Дата опрацювання"
	writeRange := fmt.Sprintf("'%s'!%s5:%s6", sheetName, columnToLetter(colIndex), columnToLetter(colIndex+1))
	_, err = s.client.Spreadsheets.Values.Update(spreadsheetID, writeRange, &sheets.ValueRange{
		Values: [][]interface{}{
			{"Вісник", ""},
			{"Дата отримання", "Дата опрацювання"},
		},
	}).ValueInputOption("USER_ENTERED").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	col0 := int64(colIndex)

	// Apply formatting to header
	requests := []*sheets.Request{
		// Set column widths for publisher columns
		{
			UpdateDimensionProperties: &sheets.UpdateDimensionPropertiesRequest{
				Range: &sheets.DimensionRange{
					SheetId:    sheetID,
					Dimension:  "COLUMNS",
					StartIndex: col0,
					EndIndex:   col0 + 2,
				},
				Properties: &sheets.DimensionProperties{
					PixelSize: colWidthPublisher,
				},
				Fields: "pixelSize",
			},
		},
		// Merge "Вісник" header (2 columns)
		{
			MergeCells: &sheets.MergeCellsRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    4, // Row 5 (0-based)
					EndRowIndex:      5,
					StartColumnIndex: col0,
					EndColumnIndex:   col0 + 2,
				},
				MergeType: "MERGE_ALL",
			},
		},
		// Format header cells - gray background, bold, borders
		{
			RepeatCell: &sheets.RepeatCellRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    4,
					EndRowIndex:      6,
					StartColumnIndex: col0,
					EndColumnIndex:   col0 + 2,
				},
				Cell: &sheets.CellData{
					UserEnteredFormat: &sheets.CellFormat{
						BackgroundColor: hexToColor(headerBgColor),
						TextFormat: &sheets.TextFormat{
							Bold:            true,
							FontSize:        9,
							ForegroundColor: hexToColor(headerTextColor),
						},
						HorizontalAlignment: "CENTER",
						VerticalAlignment:   "MIDDLE",
						Borders: &sheets.Borders{
							Top:    &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Bottom: &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Left:   &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Right:  &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
						},
					},
				},
				Fields: "userEnteredFormat(backgroundColor,textFormat,horizontalAlignment,verticalAlignment,borders)",
			},
		},
		// Add thick right border after this publisher column in header (Дата опрацювання column)
		{
			RepeatCell: &sheets.RepeatCellRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    4,
					EndRowIndex:      6,
					StartColumnIndex: col0 + 1,
					EndColumnIndex:   col0 + 2,
				},
				Cell: &sheets.CellData{
					UserEnteredFormat: &sheets.CellFormat{
						BackgroundColor: hexToColor(headerBgColor),
						TextFormat: &sheets.TextFormat{
							Bold:            true,
							FontSize:        9,
							ForegroundColor: hexToColor(headerTextColor),
						},
						HorizontalAlignment: "CENTER",
						VerticalAlignment:   "MIDDLE",
						Borders: &sheets.Borders{
							Top:    &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Bottom: &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Left:   &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Right:  &sheets.Border{Style: "SOLID_THICK", Color: hexToColor(0x000000)},
						},
					},
				},
				Fields: "userEnteredFormat(backgroundColor,textFormat,horizontalAlignment,verticalAlignment,borders)",
			},
		},
	}

	_, err = s.client.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: requests,
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to format header: %w", err)
	}

	return nil
}

// formatPublisherEntry applies formatting to a publisher entry (merge cells, borders)
func (s *Service) formatPublisherEntry(ctx context.Context, spreadsheetID string, sheetID int64, rowIndex, colIndex int) error {
	// Convert to 0-based indices for API
	row0 := int64(rowIndex - 1)
	col0 := int64(colIndex)

	requests := []*sheets.Request{
		// Merge publisher name cells (2 columns on row 1)
		{
			MergeCells: &sheets.MergeCellsRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    row0,
					EndRowIndex:      row0 + 1,
					StartColumnIndex: col0,
					EndColumnIndex:   col0 + 2,
				},
				MergeType: "MERGE_ALL",
			},
		},
		// Format the entry cells - all borders, centered, bold
		{
			RepeatCell: &sheets.RepeatCellRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    row0,
					EndRowIndex:      row0 + 2,
					StartColumnIndex: col0,
					EndColumnIndex:   col0 + 2,
				},
				Cell: &sheets.CellData{
					UserEnteredFormat: &sheets.CellFormat{
						TextFormat: &sheets.TextFormat{
							Bold:     true,
							FontSize: 11,
						},
						HorizontalAlignment: "CENTER",
						VerticalAlignment:   "MIDDLE",
						Borders: &sheets.Borders{
							Top:    &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Bottom: &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Left:   &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Right:  &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
						},
					},
				},
				Fields: "userEnteredFormat(textFormat,horizontalAlignment,verticalAlignment,borders)",
			},
		},
		// Add thick right border after the publisher entry (Дата опрацювання column)
		{
			RepeatCell: &sheets.RepeatCellRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    row0,
					EndRowIndex:      row0 + 2,
					StartColumnIndex: col0 + 1,
					EndColumnIndex:   col0 + 2,
				},
				Cell: &sheets.CellData{
					UserEnteredFormat: &sheets.CellFormat{
						TextFormat: &sheets.TextFormat{
							Bold:     true,
							FontSize: 11,
						},
						HorizontalAlignment: "CENTER",
						VerticalAlignment:   "MIDDLE",
						Borders: &sheets.Borders{
							Top:    &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Bottom: &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Left:   &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Right:  &sheets.Border{Style: "SOLID_THICK", Color: hexToColor(0x000000)},
						},
					},
				},
				Fields: "userEnteredFormat(textFormat,horizontalAlignment,verticalAlignment,borders)",
			},
		},
	}

	_, err := s.client.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: requests,
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to format publisher entry: %w", err)
	}

	return nil
}

// RecordTerritoryReturn records when a territory is returned
// In the new format: return date goes in the second column of row 2
func (s *Service) RecordTerritoryReturn(spreadsheetID, territoryTitle, groupTitle, publisherName string, dateReturned time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	sheetName := GetServiceYear(dateReturned)

	// Find the territory row by both title AND group (composite key)
	rowIndex, err := s.findTerritoryRow(ctx, spreadsheetID, sheetName, territoryTitle, groupTitle)
	if err != nil {
		return fmt.Errorf("failed to find territory row: %w", err)
	}

	if rowIndex == -1 {
		return fmt.Errorf("territory %s in group %s not found in sheet %s", territoryTitle, groupTitle, sheetName)
	}

	// Find the column with the publisher's take date that doesn't have a return date
	returnCol, err := s.findReturnColumn(ctx, spreadsheetID, sheetName, rowIndex, publisherName)
	if err != nil {
		return fmt.Errorf("failed to find return column: %w", err)
	}

	if returnCol == -1 {
		return fmt.Errorf("could not find open take record for publisher %s on territory %s", publisherName, territoryTitle)
	}

	// Write the return date on row 2 (rowIndex + 1)
	dateStr := dateReturned.Format("02.01.2006")
	writeRange := fmt.Sprintf("'%s'!%s%d", sheetName, columnToLetter(returnCol), rowIndex+1)

	_, err = s.client.Spreadsheets.Values.Update(spreadsheetID, writeRange, &sheets.ValueRange{
		Values: [][]interface{}{{dateStr}},
	}).ValueInputOption("USER_ENTERED").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to write return date: %w", err)
	}

	// Update the "Остання дата опрацювання" column (C) with the return date
	lastDateRange := fmt.Sprintf("'%s'!C%d", sheetName, rowIndex)
	_, err = s.client.Spreadsheets.Values.Update(spreadsheetID, lastDateRange, &sheets.ValueRange{
		Values: [][]interface{}{{dateStr}},
	}).ValueInputOption("USER_ENTERED").Context(ctx).Do()
	if err != nil {
		// Log but don't fail - return date was already written
	}

	return nil
}

// UpdateLastProcessedDate updates only the "Остання дата опрацювання" column for a territory
// Used for migration when territory is not currently in use
func (s *Service) UpdateLastProcessedDate(spreadsheetID, territoryTitle, groupTitle string, lastDate time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	sheetName := GetServiceYear(lastDate)

	// Find the territory row
	rowIndex, err := s.findTerritoryRow(ctx, spreadsheetID, sheetName, territoryTitle, groupTitle)
	if err != nil {
		return fmt.Errorf("failed to find territory row: %w", err)
	}
	if rowIndex == -1 {
		return fmt.Errorf("territory not found: %s / %s", territoryTitle, groupTitle)
	}

	// Update column C (Остання дата опрацювання)
	dateRange := fmt.Sprintf("'%s'!C%d", sheetName, rowIndex)
	dateStr := lastDate.Format("02.01.2006")

	_, err = s.client.Spreadsheets.Values.Update(spreadsheetID, dateRange, &sheets.ValueRange{
		Values: [][]interface{}{{dateStr}},
	}).ValueInputOption("USER_ENTERED").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to update last processed date: %w", err)
	}

	return nil
}

// RecordTerritoryTakeWithoutReturn records a territory take without return date
// Used for migration when territory is currently in use
func (s *Service) RecordTerritoryTakeWithoutReturn(spreadsheetID, territoryTitle, groupTitle, publisherName string, dateTaken time.Time) error {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	sheetName := GetServiceYear(dateTaken)

	// Ensure the sheet exists
	if err := s.ensureSheetExists(ctx, spreadsheetID, sheetName); err != nil {
		return fmt.Errorf("failed to ensure sheet exists: %w", err)
	}

	// Find the territory row by both title AND group (composite key)
	rowIndex, err := s.findTerritoryRow(ctx, spreadsheetID, sheetName, territoryTitle, groupTitle)
	if err != nil {
		return fmt.Errorf("failed to find territory row: %w", err)
	}

	// If not found, add a new row
	if rowIndex == -1 {
		rowIndex, err = s.addTerritoryRow(ctx, spreadsheetID, sheetName, territoryTitle, groupTitle)
		if err != nil {
			return fmt.Errorf("failed to add territory row: %w", err)
		}
	}

	// Find the next available column for this territory
	nextCol, err := s.findNextAvailableColumn(ctx, spreadsheetID, sheetName, rowIndex)
	if err != nil {
		return fmt.Errorf("failed to find next column: %w", err)
	}

	// Check if we need to add header for new publisher column
	sheetID, err := s.getSheetID(ctx, spreadsheetID, sheetName)
	if err != nil {
		return fmt.Errorf("failed to get sheet ID: %w", err)
	}

	// Add header if this is a new publisher column (check if header exists)
	if nextCol >= colFirstVisnik {
		headerRange := fmt.Sprintf("'%s'!%s5", sheetName, columnToLetter(nextCol))
		resp, err := s.client.Spreadsheets.Values.Get(spreadsheetID, headerRange).Context(ctx).Do()
		if err != nil || len(resp.Values) == 0 || len(resp.Values[0]) == 0 || resp.Values[0][0] == "" {
			// Need to add header
			if err := s.addPublisherHeader(ctx, spreadsheetID, sheetName, sheetID, nextCol); err != nil {
				return fmt.Errorf("failed to add publisher header: %w", err)
			}
		}
	}

	// Write publisher name (merged across 2 columns) and take date
	dateStr := dateTaken.Format("02.01.2006")
	publisherRange := fmt.Sprintf("'%s'!%s%d:%s%d", sheetName, columnToLetter(nextCol), rowIndex, columnToLetter(nextCol+1), rowIndex+1)

	_, err = s.client.Spreadsheets.Values.Update(spreadsheetID, publisherRange, &sheets.ValueRange{
		Values: [][]interface{}{
			{publisherName, ""},           // Row 1: Publisher name (merged)
			{dateStr, ""},                 // Row 2: Take date, empty return date
		},
	}).ValueInputOption("USER_ENTERED").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to write territory take: %w", err)
	}

	// Format the entry cells
	if err := s.formatPublisherEntry(ctx, spreadsheetID, sheetID, rowIndex, nextCol); err != nil {
		return fmt.Errorf("failed to format entry: %w", err)
	}

	return nil
}

// GetSpreadsheetLink returns the link to the spreadsheet
func GetSpreadsheetLink(spreadsheetID string) string {
	return fmt.Sprintf("https://docs.google.com/spreadsheets/d/%s", spreadsheetID)
}

// SheetExists checks if a sheet with the given name exists in the spreadsheet
func (s *Service) SheetExists(spreadsheetID, sheetName string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	spreadsheet, err := s.client.Spreadsheets.Get(spreadsheetID).Context(ctx).Do()
	if err != nil {
		return false, fmt.Errorf("failed to get spreadsheet: %w", err)
	}

	for _, sheet := range spreadsheet.Sheets {
		if sheet.Properties.Title == sheetName {
			return true, nil
		}
	}

	return false, nil
}

// ensureSheetExists creates the sheet if it doesn't exist with proper formatting
func (s *Service) ensureSheetExists(ctx context.Context, spreadsheetID, sheetName string) error {
	// Get spreadsheet metadata
	spreadsheet, err := s.client.Spreadsheets.Get(spreadsheetID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to get spreadsheet: %w", err)
	}

	// Check if sheet exists
	for _, sheet := range spreadsheet.Sheets {
		if sheet.Properties.Title == sheetName {
			return nil
		}
	}

	// Create the sheet
	resp, err := s.client.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				AddSheet: &sheets.AddSheetRequest{
					Properties: &sheets.SheetProperties{
						Title: sheetName,
					},
				},
			},
		},
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to create sheet: %w", err)
	}

	// Get the new sheet ID
	var sheetID int64
	for _, reply := range resp.Replies {
		if reply.AddSheet != nil {
			sheetID = reply.AddSheet.Properties.SheetId
			break
		}
	}

	// Setup the sheet with formatting
	return s.setupSheetFormatting(ctx, spreadsheetID, sheetName, sheetID)
}

// setupSheetFormatting applies the template formatting to a new sheet
func (s *Service) setupSheetFormatting(ctx context.Context, spreadsheetID, sheetName string, sheetID int64) error {
	// Write header content - only static headers, no pre-filled Вісник columns
	// Columns: A=№ Тер., B=Місце, C=Остання дата, D+=Publisher entries (added dynamically)
	headerRange := fmt.Sprintf("'%s'!A1:C6", sheetName)
	_, err := s.client.Spreadsheets.Values.Update(spreadsheetID, headerRange, &sheets.ValueRange{
		Values: [][]interface{}{
			{"ЗАПИСИ ПРО ОПРАЦЮВАННЯ ТЕРИТОРІЙ"},
			{},
			{fmt.Sprintf("Службовий рік: %s", sheetName)},
			{},
			{"№ Тер.", "Місце", "Остання дата опрацювання"},
			{"", "", ""},
		},
	}).ValueInputOption("USER_ENTERED").Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to write headers: %w", err)
	}

	// Apply formatting
	requests := []*sheets.Request{
		// Set column width for № Тер. (A)
		{
			UpdateDimensionProperties: &sheets.UpdateDimensionPropertiesRequest{
				Range: &sheets.DimensionRange{
					SheetId:    sheetID,
					Dimension:  "COLUMNS",
					StartIndex: 0,
					EndIndex:   1,
				},
				Properties: &sheets.DimensionProperties{
					PixelSize: colWidthTerritoryNum,
				},
				Fields: "pixelSize",
			},
		},
		// Set column width for Місце (B)
		{
			UpdateDimensionProperties: &sheets.UpdateDimensionPropertiesRequest{
				Range: &sheets.DimensionRange{
					SheetId:    sheetID,
					Dimension:  "COLUMNS",
					StartIndex: 1,
					EndIndex:   2,
				},
				Properties: &sheets.DimensionProperties{
					PixelSize: colWidthPlace,
				},
				Fields: "pixelSize",
			},
		},
		// Set column width for Остання дата опрацювання (C)
		{
			UpdateDimensionProperties: &sheets.UpdateDimensionPropertiesRequest{
				Range: &sheets.DimensionRange{
					SheetId:    sheetID,
					Dimension:  "COLUMNS",
					StartIndex: 2,
					EndIndex:   3,
				},
				Properties: &sheets.DimensionProperties{
					PixelSize: colWidthLastDate,
				},
				Fields: "pixelSize",
			},
		},
		// Merge title row (A1:F1) - wider to fit full title
		{
			MergeCells: &sheets.MergeCellsRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    0,
					EndRowIndex:      1,
					StartColumnIndex: 0,
					EndColumnIndex:   6, // Extend to column F
				},
				MergeType: "MERGE_ALL",
			},
		},
		// Merge service year row (A3:F3) - wider to match title
		{
			MergeCells: &sheets.MergeCellsRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    2,
					EndRowIndex:      3,
					StartColumnIndex: 0,
					EndColumnIndex:   6, // Extend to column F
				},
				MergeType: "MERGE_ALL",
			},
		},
		// Merge header cells vertically (№ Тер. - A5:A6)
		{
			MergeCells: &sheets.MergeCellsRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    4,
					EndRowIndex:      6,
					StartColumnIndex: 0,
					EndColumnIndex:   1,
				},
				MergeType: "MERGE_ALL",
			},
		},
		// Merge header cells vertically (Місце - B5:B6)
		{
			MergeCells: &sheets.MergeCellsRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    4,
					EndRowIndex:      6,
					StartColumnIndex: 1,
					EndColumnIndex:   2,
				},
				MergeType: "MERGE_ALL",
			},
		},
		// Merge header cells vertically (Остання дата - C5:C6)
		{
			MergeCells: &sheets.MergeCellsRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    4,
					EndRowIndex:      6,
					StartColumnIndex: 2,
					EndColumnIndex:   3,
				},
				MergeType: "MERGE_ALL",
			},
		},
		// Format title row - bold, centered, 14pt
		{
			RepeatCell: &sheets.RepeatCellRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    0,
					EndRowIndex:      1,
					StartColumnIndex: 0,
					EndColumnIndex:   6, // Match merge range
				},
				Cell: &sheets.CellData{
					UserEnteredFormat: &sheets.CellFormat{
						TextFormat: &sheets.TextFormat{
							Bold:     true,
							FontSize: 14,
						},
						HorizontalAlignment: "CENTER",
						VerticalAlignment:   "MIDDLE",
					},
				},
				Fields: "userEnteredFormat(textFormat,horizontalAlignment,verticalAlignment)",
			},
		},
		// Format service year row - bold, centered, 12pt
		{
			RepeatCell: &sheets.RepeatCellRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    2,
					EndRowIndex:      3,
					StartColumnIndex: 0,
					EndColumnIndex:   6, // Match merge range
				},
				Cell: &sheets.CellData{
					UserEnteredFormat: &sheets.CellFormat{
						TextFormat: &sheets.TextFormat{
							Bold:     true,
							FontSize: 12,
						},
						HorizontalAlignment: "CENTER",
						VerticalAlignment:   "MIDDLE",
					},
				},
				Fields: "userEnteredFormat(textFormat,horizontalAlignment,verticalAlignment)",
			},
		},
		// Format header rows (5-6) - gray background, bold, centered, with all borders
		{
			RepeatCell: &sheets.RepeatCellRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    4,
					EndRowIndex:      6,
					StartColumnIndex: 0,
					EndColumnIndex:   3,
				},
				Cell: &sheets.CellData{
					UserEnteredFormat: &sheets.CellFormat{
						BackgroundColor: hexToColor(headerBgColor),
						TextFormat: &sheets.TextFormat{
							Bold:            true,
							FontSize:        9,
							ForegroundColor: hexToColor(headerTextColor),
						},
						HorizontalAlignment: "CENTER",
						VerticalAlignment:   "MIDDLE",
						WrapStrategy:        "WRAP",
						Borders: &sheets.Borders{
							Top:    &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Bottom: &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Left:   &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Right:  &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
						},
					},
				},
				Fields: "userEnteredFormat(backgroundColor,textFormat,horizontalAlignment,verticalAlignment,wrapStrategy,borders)",
			},
		},
		// Add thick bottom border to separator row (row 4)
		{
			RepeatCell: &sheets.RepeatCellRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    3,
					EndRowIndex:      4,
					StartColumnIndex: 0,
					EndColumnIndex:   20, // Extend across potential publisher columns
				},
				Cell: &sheets.CellData{
					UserEnteredFormat: &sheets.CellFormat{
						Borders: &sheets.Borders{
							Bottom: &sheets.Border{Style: "SOLID_THICK", Color: hexToColor(0x000000)},
						},
					},
				},
				Fields: "userEnteredFormat(borders)",
			},
		},
		// Add thick right border after Остання дата column (C) in header - include all borders to not overwrite
		{
			RepeatCell: &sheets.RepeatCellRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    4,
					EndRowIndex:      6,
					StartColumnIndex: 2,
					EndColumnIndex:   3,
				},
				Cell: &sheets.CellData{
					UserEnteredFormat: &sheets.CellFormat{
						BackgroundColor: hexToColor(headerBgColor),
						TextFormat: &sheets.TextFormat{
							Bold:            true,
							FontSize:        9,
							ForegroundColor: hexToColor(headerTextColor),
						},
						HorizontalAlignment: "CENTER",
						VerticalAlignment:   "MIDDLE",
						WrapStrategy:        "WRAP",
						Borders: &sheets.Borders{
							Top:    &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Bottom: &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Left:   &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Right:  &sheets.Border{Style: "SOLID_THICK", Color: hexToColor(0x000000)},
						},
					},
				},
				Fields: "userEnteredFormat(backgroundColor,textFormat,horizontalAlignment,verticalAlignment,wrapStrategy,borders)",
			},
		},
		// Freeze header rows
		{
			UpdateSheetProperties: &sheets.UpdateSheetPropertiesRequest{
				Properties: &sheets.SheetProperties{
					SheetId: sheetID,
					GridProperties: &sheets.GridProperties{
						FrozenRowCount: 6,
					},
				},
				Fields: "gridProperties.frozenRowCount",
			},
		},
	}

	_, err = s.client.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: requests,
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to apply formatting: %w", err)
	}

	return nil
}

// findTerritoryRow finds the row index for a territory by title AND group
// Returns the first row of the territory (each territory uses 2 rows)
// Returns -1 if not found
// Uses composite key (territory number + place/group) to uniquely identify territory
func (s *Service) findTerritoryRow(ctx context.Context, spreadsheetID, sheetName, territoryTitle, groupTitle string) (int, error) {
	// Read columns A and B (territory numbers and places) starting from row 7
	readRange := fmt.Sprintf("'%s'!A7:B500", sheetName)
	resp, err := s.client.Spreadsheets.Values.Get(spreadsheetID, readRange).Context(ctx).Do()
	if err != nil {
		return -1, fmt.Errorf("failed to read territory columns: %w", err)
	}

	// Each territory uses 2 rows, so we check every other row
	for i := 0; i < len(resp.Values); i += 2 {
		row := resp.Values[i]
		if len(row) >= 2 {
			rowTitle := fmt.Sprint(row[0])
			rowGroup := fmt.Sprint(row[1])
			// Match both territory number AND group/place
			if rowTitle == territoryTitle && rowGroup == groupTitle {
				return i + 7, nil // +7 because we start reading from row 7 (1-based)
			}
		}
	}

	return -1, nil
}

// addTerritoryRow adds a new territory row (2 rows) and returns the first row index
func (s *Service) addTerritoryRow(ctx context.Context, spreadsheetID, sheetName, territoryTitle, groupTitle string) (int, error) {
	// Read existing territories (columns A and B) to find insertion point
	readRange := fmt.Sprintf("'%s'!A7:B1000", sheetName)
	resp, err := s.client.Spreadsheets.Values.Get(spreadsheetID, readRange).Context(ctx).Do()
	if err != nil {
		return -1, fmt.Errorf("failed to read territory columns: %w", err)
	}

	// Parse existing territories and find insertion point
	// Each territory takes 2 rows, so we read every other row
	type territoryEntry struct {
		title    string
		group    string
		rowIndex int // 1-based row index in sheet
	}
	var existingTerritories []territoryEntry

	for i := 0; i < len(resp.Values); i += 2 {
		if len(resp.Values[i]) >= 2 && resp.Values[i][0] != nil && resp.Values[i][0] != "" {
			title := fmt.Sprintf("%v", resp.Values[i][0])
			group := ""
			if resp.Values[i][1] != nil {
				group = fmt.Sprintf("%v", resp.Values[i][1])
			}
			existingTerritories = append(existingTerritories, territoryEntry{
				title:    title,
				group:    group,
				rowIndex: 7 + i, // 7 is first data row (1-based)
			})
		}
	}

	// Find the correct insertion point (sorted by group, then by title)
	insertionRowIndex := 7 // Default: first data row
	territoryCount := len(existingTerritories)

	if territoryCount > 0 {
		// Find where to insert based on group and title ordering
		insertAfterIdx := -1 // Index in existingTerritories array

		for idx, entry := range existingTerritories {
			// Compare groups first
			if entry.group < groupTitle {
				insertAfterIdx = idx
			} else if entry.group == groupTitle {
				// Same group - compare titles naturally
				if naturalLessThan(entry.title, territoryTitle) {
					insertAfterIdx = idx
				}
			}
		}

		if insertAfterIdx == -1 {
			// Insert at the very beginning (before all existing)
			insertionRowIndex = 7
		} else {
			// Insert after the found territory
			insertionRowIndex = existingTerritories[insertAfterIdx].rowIndex + 2
		}
	}

	// Get sheet ID for insertion
	sheetID, err := s.getSheetID(ctx, spreadsheetID, sheetName)
	if err != nil {
		return -1, fmt.Errorf("failed to get sheet ID: %w", err)
	}

	// If not inserting at the end, we need to insert new rows
	endRowIndex := 7 + (territoryCount * 2)
	if insertionRowIndex < endRowIndex {
		// Insert 2 new rows at the insertion point
		_, err = s.client.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{
				{
					InsertDimension: &sheets.InsertDimensionRequest{
						Range: &sheets.DimensionRange{
							SheetId:    sheetID,
							Dimension:  "ROWS",
							StartIndex: int64(insertionRowIndex - 1), // 0-based
							EndIndex:   int64(insertionRowIndex + 1), // Insert 2 rows
						},
						InheritFromBefore: false,
					},
				},
			},
		}).Context(ctx).Do()
		if err != nil {
			return -1, fmt.Errorf("failed to insert rows: %w", err)
		}
	}

	// Write the new territory data (2 rows)
	writeRange := fmt.Sprintf("'%s'!A%d:C%d", sheetName, insertionRowIndex, insertionRowIndex+1)
	_, err = s.client.Spreadsheets.Values.Update(spreadsheetID, writeRange, &sheets.ValueRange{
		Values: [][]interface{}{
			{territoryTitle, groupTitle, ""},
			{"", "", ""},
		},
	}).ValueInputOption("USER_ENTERED").Context(ctx).Do()
	if err != nil {
		return -1, fmt.Errorf("failed to write territory row: %w", err)
	}

	err = s.formatTerritoryRow(ctx, spreadsheetID, sheetID, insertionRowIndex)
	if err != nil {
		// Log error but don't fail - data is already written
		return insertionRowIndex, nil
	}

	return insertionRowIndex, nil
}

// naturalLessThan compares two strings naturally (so "2" < "10")
func naturalLessThan(a, b string) bool {
	numA, restA := extractNumber(a)
	numB, restB := extractNumber(b)

	if numA != -1 && numB != -1 {
		if numA != numB {
			return numA < numB
		}
		return restA < restB
	}
	return a < b
}

// extractNumber extracts leading number from string, returns -1 if none
func extractNumber(s string) (int, string) {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return -1, s
	}
	num := 0
	fmt.Sscanf(s[:i], "%d", &num)
	return num, s[i:]
}

// formatTerritoryRow applies formatting to a territory row pair
func (s *Service) formatTerritoryRow(ctx context.Context, spreadsheetID string, sheetID int64, rowIndex int) error {
	// Convert to 0-based index for API
	row0 := int64(rowIndex - 1)

	requests := []*sheets.Request{
		// Merge territory number cells (A row1:A row2)
		{
			MergeCells: &sheets.MergeCellsRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    row0,
					EndRowIndex:      row0 + 2,
					StartColumnIndex: 0,
					EndColumnIndex:   1,
				},
				MergeType: "MERGE_ALL",
			},
		},
		// Merge place cells (B row1:B row2)
		{
			MergeCells: &sheets.MergeCellsRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    row0,
					EndRowIndex:      row0 + 2,
					StartColumnIndex: 1,
					EndColumnIndex:   2,
				},
				MergeType: "MERGE_ALL",
			},
		},
		// Merge last date cells (C row1:C row2)
		{
			MergeCells: &sheets.MergeCellsRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    row0,
					EndRowIndex:      row0 + 2,
					StartColumnIndex: 2,
					EndColumnIndex:   3,
				},
				MergeType: "MERGE_ALL",
			},
		},
		// Format the static columns (A-C) - all borders, centered, bold
		{
			RepeatCell: &sheets.RepeatCellRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    row0,
					EndRowIndex:      row0 + 2,
					StartColumnIndex: 0,
					EndColumnIndex:   3,
				},
				Cell: &sheets.CellData{
					UserEnteredFormat: &sheets.CellFormat{
						TextFormat: &sheets.TextFormat{
							Bold:     true,
							FontSize: 11,
						},
						HorizontalAlignment: "CENTER",
						VerticalAlignment:   "MIDDLE",
						Borders: &sheets.Borders{
							Top:    &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Bottom: &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Left:   &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Right:  &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
						},
					},
				},
				Fields: "userEnteredFormat(textFormat,horizontalAlignment,verticalAlignment,borders)",
			},
		},
		// Add thick right border after column C (last date column) - include all borders to not overwrite
		{
			RepeatCell: &sheets.RepeatCellRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    row0,
					EndRowIndex:      row0 + 2,
					StartColumnIndex: 2,
					EndColumnIndex:   3,
				},
				Cell: &sheets.CellData{
					UserEnteredFormat: &sheets.CellFormat{
						TextFormat: &sheets.TextFormat{
							Bold:     true,
							FontSize: 11,
						},
						HorizontalAlignment: "CENTER",
						VerticalAlignment:   "MIDDLE",
						Borders: &sheets.Borders{
							Top:    &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Bottom: &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Left:   &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Right:  &sheets.Border{Style: "SOLID_THICK", Color: hexToColor(0x000000)},
						},
					},
				},
				Fields: "userEnteredFormat(textFormat,horizontalAlignment,verticalAlignment,borders)",
			},
		},
	}

	_, err := s.client.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: requests,
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to format territory row: %w", err)
	}

	return nil
}

// findNextAvailableColumn finds the next empty column pair for a territory row
// In the new format: A=Territory#, B=Place, C=LastDate, D+=Publisher entries
// Each publisher uses 2 columns: merged name on row1, dates on row2
func (s *Service) findNextAvailableColumn(ctx context.Context, spreadsheetID, sheetName string, rowIndex int) (int, error) {
	// Read both rows of the territory (publisher names are on first row)
	readRange := fmt.Sprintf("'%s'!A%d:ZZ%d", sheetName, rowIndex, rowIndex)
	resp, err := s.client.Spreadsheets.Values.Get(spreadsheetID, readRange).Context(ctx).Do()
	if err != nil {
		return -1, fmt.Errorf("failed to read row: %w", err)
	}

	// Start from column D (index 3) - first publisher column
	// Each publisher entry uses 2 columns
	startCol := colFirstVisnik

	if len(resp.Values) == 0 || len(resp.Values[0]) <= startCol {
		return startCol, nil
	}

	row := resp.Values[0]

	// Find the first empty column pair starting from column D
	// We look for empty cells in groups of 2 (publisher name merged across 2 cols)
	for col := startCol; col < len(row); col += colsPerPublisher {
		// If this column is empty, it's available
		if col >= len(row) || row[col] == nil || row[col] == "" {
			return col, nil
		}
	}

	// If all existing columns are filled, return the next one
	nextCol := len(row)
	// Ensure we're aligned to start of a new publisher group (2 columns)
	if (nextCol-startCol)%colsPerPublisher != 0 {
		nextCol = startCol + ((nextCol-startCol)/colsPerPublisher+1)*colsPerPublisher
	}

	return nextCol, nil
}

// findReturnColumn finds the column where the return date should be written
// In the new format: publisher name on row1 (merged), dates on row2
// Row2 has: DateTaken (col), DateReturned (col+1)
func (s *Service) findReturnColumn(ctx context.Context, spreadsheetID, sheetName string, rowIndex int, publisherName string) (int, error) {
	// Read both rows of the territory
	readRange := fmt.Sprintf("'%s'!A%d:ZZ%d", sheetName, rowIndex, rowIndex+1)
	resp, err := s.client.Spreadsheets.Values.Get(spreadsheetID, readRange).Context(ctx).Do()
	if err != nil {
		return -1, fmt.Errorf("failed to read rows: %w", err)
	}

	if len(resp.Values) < 2 {
		return -1, nil
	}

	row1 := resp.Values[0] // Publisher names
	row2 := resp.Values[1] // Dates

	startCol := colFirstVisnik // Column D

	// Look for publisher name on row1, check if return date on row2 is empty
	// Pattern: Row1 has publisher name (merged across 2 cols)
	// Row2 has: DateTaken (first col), DateReturned (second col)
	for col := startCol; col < len(row1); col += colsPerPublisher {
		if col >= len(row1) {
			break
		}

		// Check if this is the publisher
		if fmt.Sprint(row1[col]) == publisherName {
			// Return date is in the second column of the pair on row2
			returnDateCol := col + 1
			if returnDateCol >= len(row2) || row2[returnDateCol] == nil || row2[returnDateCol] == "" {
				return returnDateCol, nil
			}
		}
	}

	return -1, nil
}

// columnToLetter converts a 0-based column index to a letter (A, B, ..., Z, AA, AB, ...)
func columnToLetter(col int) string {
	result := ""
	for col >= 0 {
		result = string(rune('A'+col%26)) + result
		col = col/26 - 1
	}
	return result
}

// TerritorySetup represents a territory for bulk setup
type TerritorySetup struct {
	Number string // Territory number (e.g., "1", "2", "3")
	Place  string // Place/Group name (e.g., "Klevan", "Rivne")
}

// SetupTerritories creates a formatted sheet with pre-populated territory rows
// This is useful for initial setup of a new congregation's spreadsheet
func (s *Service) SetupTerritories(spreadsheetID string, territories []TerritorySetup, serviceYear time.Time) error {
	ctx := context.Background()
	sheetName := GetServiceYear(serviceYear)

	// Delete the sheet if it exists and recreate it
	spreadsheet, err := s.client.Spreadsheets.Get(spreadsheetID).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to get spreadsheet: %w", err)
	}

	var existingSheetID int64 = -1
	for _, sheet := range spreadsheet.Sheets {
		if sheet.Properties.Title == sheetName {
			existingSheetID = sheet.Properties.SheetId
			break
		}
	}

	// Delete existing sheet if found
	if existingSheetID != -1 {
		_, err = s.client.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
			Requests: []*sheets.Request{
				{
					DeleteSheet: &sheets.DeleteSheetRequest{
						SheetId: existingSheetID,
					},
				},
			},
		}).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("failed to delete existing sheet: %w", err)
		}
	}

	// Create new sheet with formatting
	resp, err := s.client.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
		Requests: []*sheets.Request{
			{
				AddSheet: &sheets.AddSheetRequest{
					Properties: &sheets.SheetProperties{
						Title: sheetName,
					},
				},
			},
		},
	}).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("failed to create sheet: %w", err)
	}

	var sheetID int64
	for _, reply := range resp.Replies {
		if reply.AddSheet != nil {
			sheetID = reply.AddSheet.Properties.SheetId
			break
		}
	}

	// Setup header formatting
	if err := s.setupSheetFormatting(ctx, spreadsheetID, sheetName, sheetID); err != nil {
		return fmt.Errorf("failed to setup sheet formatting: %w", err)
	}

	// Write all territory data
	if len(territories) > 0 {
		// Build the values array (2 rows per territory)
		// Columns: A=Number, B=Place, C=LastDate
		var values [][]interface{}
		for _, t := range territories {
			values = append(values,
				[]interface{}{t.Number, t.Place, ""}, // Row 1: Number, Place, LastDate
				[]interface{}{"", "", ""},            // Row 2: empty (for dates)
			)
		}

		// Write territory data starting from row 7
		writeRange := fmt.Sprintf("'%s'!A7:C%d", sheetName, 7+len(values)-1)
		_, err = s.client.Spreadsheets.Values.Update(spreadsheetID, writeRange, &sheets.ValueRange{
			Values: values,
		}).ValueInputOption("USER_ENTERED").Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("failed to write territory data: %w", err)
		}

		// Apply formatting to all territory rows
		if err := s.formatAllTerritoryRows(ctx, spreadsheetID, sheetID, len(territories)); err != nil {
			return fmt.Errorf("failed to format territory rows: %w", err)
		}
	}

	return nil
}

// formatAllTerritoryRows applies formatting to all territory rows in batch
func (s *Service) formatAllTerritoryRows(ctx context.Context, spreadsheetID string, sheetID int64, territoryCount int) error {
	var requests []*sheets.Request

	for i := 0; i < territoryCount; i++ {
		row0 := int64(6 + i*2) // 0-based row index (data starts at row 7 = index 6)

		// Merge territory number cells (A row1:A row2)
		requests = append(requests, &sheets.Request{
			MergeCells: &sheets.MergeCellsRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    row0,
					EndRowIndex:      row0 + 2,
					StartColumnIndex: 0,
					EndColumnIndex:   1,
				},
				MergeType: "MERGE_ALL",
			},
		})

		// Merge place cells (B row1:B row2)
		requests = append(requests, &sheets.Request{
			MergeCells: &sheets.MergeCellsRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    row0,
					EndRowIndex:      row0 + 2,
					StartColumnIndex: 1,
					EndColumnIndex:   2,
				},
				MergeType: "MERGE_ALL",
			},
		})

		// Merge last date cells (C row1:C row2)
		requests = append(requests, &sheets.Request{
			MergeCells: &sheets.MergeCellsRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    row0,
					EndRowIndex:      row0 + 2,
					StartColumnIndex: 2,
					EndColumnIndex:   3,
				},
				MergeType: "MERGE_ALL",
			},
		})
	}

	// Format all static data columns (A-C) at once
	if territoryCount > 0 {
		dataEndRow := int64(6 + territoryCount*2)
		requests = append(requests, &sheets.Request{
			RepeatCell: &sheets.RepeatCellRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    6,
					EndRowIndex:      dataEndRow,
					StartColumnIndex: 0,
					EndColumnIndex:   3, // Columns A-C
				},
				Cell: &sheets.CellData{
					UserEnteredFormat: &sheets.CellFormat{
						TextFormat: &sheets.TextFormat{
							Bold:     true,
							FontSize: 11,
						},
						HorizontalAlignment: "CENTER",
						VerticalAlignment:   "MIDDLE",
						Borders: &sheets.Borders{
							Top:    &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Bottom: &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Left:   &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Right:  &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
						},
					},
				},
				Fields: "userEnteredFormat(textFormat,horizontalAlignment,verticalAlignment,borders)",
			},
		})

		// Add thick right border after column C (last date column) for all data rows - include all borders
		requests = append(requests, &sheets.Request{
			RepeatCell: &sheets.RepeatCellRequest{
				Range: &sheets.GridRange{
					SheetId:          sheetID,
					StartRowIndex:    6,
					EndRowIndex:      dataEndRow,
					StartColumnIndex: 2,
					EndColumnIndex:   3,
				},
				Cell: &sheets.CellData{
					UserEnteredFormat: &sheets.CellFormat{
						TextFormat: &sheets.TextFormat{
							Bold:     true,
							FontSize: 11,
						},
						HorizontalAlignment: "CENTER",
						VerticalAlignment:   "MIDDLE",
						Borders: &sheets.Borders{
							Top:    &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Bottom: &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Left:   &sheets.Border{Style: "SOLID", Color: hexToColor(0x000000)},
							Right:  &sheets.Border{Style: "SOLID_THICK", Color: hexToColor(0x000000)},
						},
					},
				},
				Fields: "userEnteredFormat(textFormat,horizontalAlignment,verticalAlignment,borders)",
			},
		})
	}

	// Execute all requests in batch
	if len(requests) > 0 {
		_, err := s.client.Spreadsheets.BatchUpdate(spreadsheetID, &sheets.BatchUpdateSpreadsheetRequest{
			Requests: requests,
		}).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("failed to apply territory formatting: %w", err)
		}
	}

	return nil
}
