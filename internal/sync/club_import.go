package sync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/jackc/pgx/v5"
)

var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)
var phonePattern = regexp.MustCompile(`^[0-9+\-\s()]{8,20}$`)

type clubImportRow struct {
	rowNumber int
	id        string
	code      string
	name      string
	phone     *string
	email     *string
	address   *string
	notes     *string
	isActive  bool
}

type clubImportColumns struct {
	nameIndex     int
	phoneIndex    int
	emailIndex    int
	addressIndex  int
	notesIndex    int
	isActiveIndex int
}

func (s *Service) ImportClubs(ctx context.Context, file io.Reader) (ImportClubsResponse, error) {
	workbook, err := excelWorkbookFromReader(file)
	if err != nil {
		return ImportClubsResponse{}, err
	}
	defer workbook.Close()

	rows, err := readFirstWorksheetRows(workbook)
	if err != nil {
		return ImportClubsResponse{}, err
	}

	columns, err := resolveClubImportColumns(rows[0])
	if err != nil {
		return ImportClubsResponse{}, err
	}

	parsedRows, parseErrors := parseClubImportRows(rows[1:], columns)
	if len(parsedRows) == 0 {
		return ImportClubsResponse{ImportedCount: 0, Errors: parseErrors}, nil
	}

	tx, err := s.store.Begin(ctx)
	if err != nil {
		return ImportClubsResponse{}, err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	importedCount := 0
	rowErrors := append([]ClubImportRowError{}, parseErrors...)
	changedIDs := make([]string, 0, len(parsedRows))

	for _, row := range parsedRows {
		existing, err := s.store.GetRecordForUpdate(ctx, tx, EntityClubs, row.id)
		if err != nil {
			return ImportClubsResponse{}, err
		}

		code, err := s.resolveUniqueClubCode(ctx, tx, row.name, row.id)
		if err != nil {
			return ImportClubsResponse{}, err
		}
		row.code = code

		createdAt := now
		if existing != nil {
			var existingRecord ClubRecord
			if err := json.Unmarshal(existing.Payload, &existingRecord); err != nil {
				return ImportClubsResponse{}, err
			}
			createdAt = existingRecord.CreatedAt
		}

		record := ClubRecord{
			BaseRecord: BaseRecord{
				ID:             row.id,
				CreatedAt:      createdAt,
				UpdatedAt:      now,
				LastModifiedAt: now,
				SyncStatus:     "synced",
			},
			Code:     &row.code,
			Name:     row.name,
			Phone:    row.phone,
			Email:    row.email,
			Address:  row.address,
			Notes:    row.notes,
			IsActive: row.isActive,
		}

		payload, err := json.Marshal(record)
		if err != nil {
			return ImportClubsResponse{}, err
		}

		storedRecord := StoredRecord{
			EntityName:       EntityClubs,
			RecordID:         row.id,
			Payload:          payload,
			LastModifiedAt:   now,
			ServerModifiedAt: now,
		}

		if err := s.store.UpsertRecord(ctx, tx, storedRecord); err != nil {
			rowErrors = append(rowErrors, ClubImportRowError{
				Row:     row.rowNumber,
				Message: err.Error(),
			})
			continue
		}

		importedCount += 1
		changedIDs = append(changedIDs, row.id)
	}

	if err := tx.Commit(ctx); err != nil {
		return ImportClubsResponse{}, err
	}

	if len(changedIDs) > 0 {
		entityNames := make([]EntityName, len(changedIDs))
		for index := range changedIDs {
			entityNames[index] = EntityClubs
		}
		s.hub.BroadcastChange(entityNames, changedIDs)
	}

	return ImportClubsResponse{
		ImportedCount: importedCount,
		Errors:        rowErrors,
	}, nil
}

func resolveClubImportColumns(headerRow []string) (clubImportColumns, error) {
	columns := clubImportColumns{
		nameIndex:     -1,
		phoneIndex:    -1,
		emailIndex:    -1,
		addressIndex:  -1,
		notesIndex:    -1,
		isActiveIndex: -1,
	}

	for index, value := range headerRow {
		switch normalizeImportHeader(value) {
		case "name":
			columns.nameIndex = index
		case "phone":
			columns.phoneIndex = index
		case "email":
			columns.emailIndex = index
		case "address":
			columns.addressIndex = index
		case "notes":
			columns.notesIndex = index
		case "isactive", "active":
			columns.isActiveIndex = index
		}
	}

	if columns.nameIndex < 0 {
		return clubImportColumns{}, errors.New("excel header must include column: name")
	}

	return columns, nil
}

func parseClubImportRows(rows [][]string, columns clubImportColumns) ([]clubImportRow, []ClubImportRowError) {
	parsedRows := make([]clubImportRow, 0, len(rows))
	parseErrors := make([]ClubImportRowError, 0)
	seenNames := make(map[string]int)

	for index, row := range rows {
		rowNumber := index + 2
		if isEmptyImportRow(row) {
			continue
		}

		name := strings.TrimSpace(getImportCell(row, columns.nameIndex))
		if name == "" {
			parseErrors = append(parseErrors, ClubImportRowError{Row: rowNumber, Message: "Name is required."})
			continue
		}

		normalizedName := NormalizeSearchText(name)
		id := normalizeClubID(name)
		if previousRow, exists := seenNames[normalizedName]; exists {
			parseErrors = append(parseErrors, ClubImportRowError{
				Row:     rowNumber,
				Message: fmt.Sprintf("Duplicate club name in file (same as row %d).", previousRow),
			})
			continue
		}

		isActive, err := parseImportActive(getImportCell(row, columns.isActiveIndex))
		if err != nil {
			parseErrors = append(parseErrors, ClubImportRowError{Row: rowNumber, Message: err.Error()})
			continue
		}

		email := strings.TrimSpace(getImportCell(row, columns.emailIndex))
		if email != "" && !emailPattern.MatchString(email) {
			parseErrors = append(parseErrors, ClubImportRowError{Row: rowNumber, Message: "Email format is invalid."})
			continue
		}

		phone := strings.TrimSpace(getImportCell(row, columns.phoneIndex))
		if phone != "" && !phonePattern.MatchString(phone) {
			parseErrors = append(parseErrors, ClubImportRowError{Row: rowNumber, Message: "Phone number format is invalid."})
			continue
		}

		seenNames[normalizedName] = rowNumber
		parsedRows = append(parsedRows, clubImportRow{
			rowNumber: rowNumber,
			id:        id,
			name:      name,
			phone:     stringPtrOrNil(phone),
			email:     stringPtrOrNil(email),
			address:   stringPtrOrNil(strings.TrimSpace(getImportCell(row, columns.addressIndex))),
			notes:     stringPtrOrNil(strings.TrimSpace(getImportCell(row, columns.notesIndex))),
			isActive:  isActive,
		})
	}

	return parsedRows, parseErrors
}

func (s *Service) resolveUniqueClubCode(ctx context.Context, tx pgx.Tx, name string, excludeID string) (string, error) {
	baseCode := generateClubCode(name)
	candidate := baseCode
	suffix := 2

	for {
		duplicated, err := s.store.FindClubByCode(ctx, tx, candidate, excludeID)
		if err != nil {
			return "", err
		}
		if duplicated == nil {
			return candidate, nil
		}

		candidate = fmt.Sprintf("%s%d", baseCode, suffix)
		suffix += 1
	}
}

func normalizeClubID(value string) string {
	base := normalizeSlug(value)
	if base == "" {
		return "club"
	}
	return base
}

func generateClubCode(value string) string {
	normalized := removeAccents(value)
	normalized = strings.TrimSpace(normalized)
	if normalized == "" {
		return "CLB"
	}

	parts := strings.FieldsFunc(normalized, func(r rune) bool {
		return !(unicode.IsLetter(r) || unicode.IsDigit(r))
	})

	var builder strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		builder.WriteRune(unicode.ToUpper([]rune(part)[0]))
	}

	if builder.Len() == 0 {
		return "CLB"
	}

	return builder.String()
}
