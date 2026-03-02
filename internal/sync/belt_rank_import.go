package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

type beltRankImportRow struct {
	rowNumber   int
	id          string
	name        string
	order       int
	description *string
	isActive    bool
}

type beltRankImportColumns struct {
	nameIndex        int
	orderIndex       int
	descriptionIndex int
	isActiveIndex    int
}

var nonSlugPattern = regexp.MustCompile(`[^a-z0-9]+`)

func (s *Service) ImportBeltRanks(ctx context.Context, file io.Reader) (ImportBeltRanksResponse, error) {
	workbook, err := excelWorkbookFromReader(file)
	if err != nil {
		return ImportBeltRanksResponse{}, err
	}
	defer workbook.Close()

	rows, err := readFirstWorksheetRows(workbook)
	if err != nil {
		return ImportBeltRanksResponse{}, err
	}

	columns, err := resolveBeltRankImportColumns(rows[0])
	if err != nil {
		return ImportBeltRanksResponse{}, err
	}

	parsedRows, parseErrors := parseBeltRankImportRows(rows[1:], columns)
	if len(parsedRows) == 0 {
		return ImportBeltRanksResponse{ImportedCount: 0, Errors: parseErrors}, nil
	}

	tx, err := s.store.Begin(ctx)
	if err != nil {
		return ImportBeltRanksResponse{}, err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	importedCount := 0
	rowErrors := append([]BeltRankImportRowError{}, parseErrors...)
	changedIDs := make([]string, 0, len(parsedRows))

	for _, row := range parsedRows {
		existing, err := s.store.GetRecordForUpdate(ctx, tx, EntityBeltRanks, row.id)
		if err != nil {
			return ImportBeltRanksResponse{}, err
		}

		duplicated, err := s.store.FindBeltRankByOrder(ctx, tx, row.order, row.id)
		if err != nil {
			return ImportBeltRanksResponse{}, err
		}
		if duplicated != nil {
			rowErrors = append(rowErrors, BeltRankImportRowError{
				Row:     row.rowNumber,
				Message: "Belt rank order already exists.",
			})
			continue
		}

		createdAt := now
		if existing != nil {
			var existingRecord BeltRankRecord
			if err := json.Unmarshal(existing.Payload, &existingRecord); err != nil {
				return ImportBeltRanksResponse{}, err
			}
			createdAt = existingRecord.CreatedAt
		}

		record := BeltRankRecord{
			BaseRecord: BaseRecord{
				ID:             row.id,
				CreatedAt:      createdAt,
				UpdatedAt:      now,
				LastModifiedAt: now,
				SyncStatus:     "synced",
			},
			Name:        row.name,
			Order:       row.order,
			Description: row.description,
			IsActive:    row.isActive,
		}

		payload, err := json.Marshal(record)
		if err != nil {
			return ImportBeltRanksResponse{}, err
		}

		storedRecord := StoredRecord{
			EntityName:       EntityBeltRanks,
			RecordID:         row.id,
			Payload:          payload,
			LastModifiedAt:   now,
			ServerModifiedAt: now,
		}

		if err := s.store.UpsertRecord(ctx, tx, storedRecord); err != nil {
			rowErrors = append(rowErrors, BeltRankImportRowError{
				Row:     row.rowNumber,
				Message: err.Error(),
			})
			continue
		}

		importedCount += 1
		changedIDs = append(changedIDs, row.id)
	}

	if err := tx.Commit(ctx); err != nil {
		return ImportBeltRanksResponse{}, err
	}

	if len(changedIDs) > 0 {
		entityNames := make([]EntityName, len(changedIDs))
		for index := range changedIDs {
			entityNames[index] = EntityBeltRanks
		}
		s.hub.BroadcastChange(entityNames, changedIDs)
	}

	return ImportBeltRanksResponse{
		ImportedCount: importedCount,
		Errors:        rowErrors,
	}, nil
}

func resolveBeltRankImportColumns(headerRow []string) (beltRankImportColumns, error) {
	columns := beltRankImportColumns{
		nameIndex:        -1,
		orderIndex:       -1,
		descriptionIndex: -1,
		isActiveIndex:    -1,
	}

	for index, value := range headerRow {
		switch normalizeImportHeader(value) {
		case "name":
			columns.nameIndex = index
		case "order", "rankorder":
			columns.orderIndex = index
		case "description":
			columns.descriptionIndex = index
		case "isactive", "active":
			columns.isActiveIndex = index
		}
	}

	if columns.nameIndex < 0 || columns.orderIndex < 0 {
		return beltRankImportColumns{}, errors.New("excel header must include columns: name, order")
	}

	return columns, nil
}

func parseBeltRankImportRows(rows [][]string, columns beltRankImportColumns) ([]beltRankImportRow, []BeltRankImportRowError) {
	parsedRows := make([]beltRankImportRow, 0, len(rows))
	errors := make([]BeltRankImportRowError, 0)
	seenIDs := make(map[string]int)
	seenOrders := make(map[int]int)

	for index, row := range rows {
		rowNumber := index + 2
		if isEmptyImportRow(row) {
			continue
		}

		name := strings.TrimSpace(getImportCell(row, columns.nameIndex))
		if name == "" {
			errors = append(errors, BeltRankImportRowError{Row: rowNumber, Message: "Name is required."})
			continue
		}

		order, err := parseImportOrder(getImportCell(row, columns.orderIndex))
		if err != nil {
			errors = append(errors, BeltRankImportRowError{Row: rowNumber, Message: err.Error()})
			continue
		}

		id := normalizeBeltRankID(name)
		if previousRow, exists := seenIDs[id]; exists {
			errors = append(errors, BeltRankImportRowError{
				Row:     rowNumber,
				Message: fmt.Sprintf("Duplicate belt rank name in file (same as row %d).", previousRow),
			})
			continue
		}
		if previousRow, exists := seenOrders[order]; exists {
			errors = append(errors, BeltRankImportRowError{
				Row:     rowNumber,
				Message: fmt.Sprintf("Duplicate belt rank order in file (same as row %d).", previousRow),
			})
			continue
		}

		var description *string
		descriptionValue := strings.TrimSpace(getImportCell(row, columns.descriptionIndex))
		if descriptionValue != "" {
			description = &descriptionValue
		}

		isActive, err := parseImportActive(getImportCell(row, columns.isActiveIndex))
		if err != nil {
			errors = append(errors, BeltRankImportRowError{Row: rowNumber, Message: err.Error()})
			continue
		}

		seenIDs[id] = rowNumber
		seenOrders[order] = rowNumber
		parsedRows = append(parsedRows, beltRankImportRow{
			rowNumber:   rowNumber,
			id:          id,
			name:        name,
			order:       order,
			description: description,
			isActive:    isActive,
		})
	}

	return parsedRows, errors
}

func normalizeImportHeader(value string) string {
	return strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(value), "_", ""), " ", ""))
}

func getImportCell(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return row[index]
}

func isEmptyImportRow(row []string) bool {
	for _, value := range row {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

func parseImportOrder(value string) (int, error) {
	rawValue := strings.TrimSpace(value)
	if rawValue == "" {
		return 0, errors.New("Order is required.")
	}

	floatValue, err := strconv.ParseFloat(rawValue, 64)
	if err != nil {
		return 0, errors.New("Order must be a valid number.")
	}
	if math.Trunc(floatValue) != floatValue || floatValue < 1 {
		return 0, errors.New("Order must be an integer greater than or equal to 1.")
	}

	return int(floatValue), nil
}

func parseImportActive(value string) (bool, error) {
	rawValue := strings.TrimSpace(strings.ToLower(value))
	if rawValue == "" {
		return true, nil
	}

	switch rawValue {
	case "true", "1", "yes", "y", "active":
		return true, nil
	case "false", "0", "no", "n", "inactive":
		return false, nil
	default:
		return false, errors.New("Active must be one of: true, false, 1, 0, yes, no.")
	}
}

func normalizeBeltRankID(value string) string {
	normalized := normalizeSlug(value)
	if normalized == "" {
		return "belt-rank"
	}
	return normalized
}

func excelWorkbookFromReader(file io.Reader) (*excelize.File, error) {
	workbook, err := excelize.OpenReader(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read excel file: %w", err)
	}
	return workbook, nil
}

func readFirstWorksheetRows(workbook *excelize.File) ([][]string, error) {
	sheets := workbook.GetSheetList()
	if len(sheets) == 0 {
		return nil, errors.New("excel file does not contain any worksheet")
	}

	rows, err := workbook.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("failed to read worksheet rows: %w", err)
	}
	if len(rows) < 2 {
		return nil, errors.New("excel file must include a header row and at least one data row")
	}

	return rows, nil
}

func normalizeSlug(value string) string {
	normalized := removeAccents(strings.ToLower(strings.TrimSpace(value)))
	normalized = nonSlugPattern.ReplaceAllString(normalized, "-")
	normalized = strings.Trim(normalized, "-")
	return normalized
}

func stringPtrOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func removeAccents(value string) string {
	transformer := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	normalized, _, err := transform.String(transformer, value)
	if err != nil {
		return value
	}
	return normalized
}
