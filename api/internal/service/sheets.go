package service

import "time"

// SheetsService defines the interface for Google Sheets operations used by the bot service.
type SheetsService interface {
	RecordTerritoryTake(spreadsheetID, territoryTitle, groupTitle, publisherName string, dateTaken time.Time) error
	RecordTerritoryReturn(spreadsheetID, territoryTitle, groupTitle, publisherName string, dateReturned time.Time) error
}

// noopSheetsService is used when Google Sheets credentials are not configured (e.g. staging).
type noopSheetsService struct{}

func NewNoopSheetsService() SheetsService                                            { return noopSheetsService{} }
func (noopSheetsService) RecordTerritoryTake(_, _, _, _ string, _ time.Time) error   { return nil }
func (noopSheetsService) RecordTerritoryReturn(_, _, _, _ string, _ time.Time) error { return nil }
