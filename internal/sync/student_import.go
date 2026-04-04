package sync

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"pqq/be/internal/auth"

	"github.com/xuri/excelize/v2"
)

type studentImportRow struct {
	rowNumber    int
	id           string
	studentCode  *string
	fullName     string
	dateOfBirth  *string
	gender       *string
	phone        *string
	email        *string
	address      *string
	clubName     string
	groupName    *string
	beltRankName string
	joinedAt     *string
	status       string
	notes        *string
	scheduleMode string
	scheduleDays []string
}

type studentImportColumns struct {
	studentCodeIndex  int
	fullNameIndex     int
	dateOfBirthIndex  int
	genderIndex       int
	phoneIndex        int
	emailIndex        int
	addressIndex      int
	clubIndex         int
	groupIndex        int
	beltRankIndex     int
	joinedAtIndex     int
	statusIndex       int
	notesIndex        int
	scheduleModeIndex int
	scheduleDaysIndex int
}

func (s *Service) ImportStudents(ctx context.Context, file io.Reader) (ImportStudentsResponse, error) {
	claims, ok := auth.ClaimsFromContext(ctx)
	if !ok || claims == nil || claims.Subject == "" {
		return ImportStudentsResponse{}, errors.New("unauthorized")
	}
	workbook, err := excelWorkbookFromReader(file)
	if err != nil {
		return ImportStudentsResponse{}, err
	}
	defer workbook.Close()

	rows, err := readFirstWorksheetRows(workbook)
	if err != nil {
		return ImportStudentsResponse{}, err
	}

	columns, err := resolveStudentImportColumns(rows[0])
	if err != nil {
		return ImportStudentsResponse{}, err
	}

	parsedRows, parseErrors := parseStudentImportRows(rows[1:], columns)
	if len(parsedRows) == 0 {
		return ImportStudentsResponse{ImportedCount: 0, Errors: parseErrors}, nil
	}

	tx, err := s.store.Begin(ctx)
	if err != nil {
		return ImportStudentsResponse{}, err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	importedCount := 0
	rowErrors := append([]StudentImportRowError{}, parseErrors...)
	changedEntityNames := make([]EntityName, 0, len(parsedRows)*3)
	changedIDs := make([]string, 0, len(parsedRows)*3)
	permissionCache := make(map[string]map[string]bool)

	for _, row := range parsedRows {
		clubRecord, err := s.store.FindClubByCode(ctx, tx, row.clubName, "")
		if err != nil {
			return ImportStudentsResponse{}, err
		}
		if clubRecord == nil {
			clubRecord, err = s.store.FindClubByName(ctx, tx, row.clubName)
			if err != nil {
				return ImportStudentsResponse{}, err
			}
		}
		if clubRecord == nil {
			rowErrors = append(rowErrors, StudentImportRowError{Row: row.rowNumber, Message: "Club does not exist by code or name."})
			continue
		}

		var club ClubRecord
		if err := json.Unmarshal(clubRecord.Payload, &club); err != nil {
			return ImportStudentsResponse{}, err
		}
		if claims.SystemRole != auth.SystemRoleSysAdmin {
			permissions, ok := permissionCache[club.ID]
			if !ok {
				response, err := s.authorizer.GetClubPermissions(ctx, claims.Subject, club.ID)
				if err != nil {
					return ImportStudentsResponse{}, err
				}
				permissions = response.Permissions
				permissionCache[club.ID] = permissions
			}
			if !permissions[auth.PermissionImportsManage] {
				rowErrors = append(rowErrors, StudentImportRowError{Row: row.rowNumber, Message: "You do not have permission to import students into this club."})
				continue
			}
		}

		var groupID *string
		if row.groupName != nil && *row.groupName != "" {
			groupRecord, err := s.store.FindClubGroupByName(ctx, tx, club.ID, *row.groupName, "")
			if err != nil {
				return ImportStudentsResponse{}, err
			}
			if groupRecord == nil {
				rowErrors = append(rowErrors, StudentImportRowError{Row: row.rowNumber, Message: "Group does not exist in the selected club."})
				continue
			}
			groupID = &groupRecord.RecordID
		}

		beltRankRecord, err := s.store.FindBeltRankByName(ctx, tx, row.beltRankName)
		if err != nil {
			return ImportStudentsResponse{}, err
		}
		if beltRankRecord == nil {
			rowErrors = append(rowErrors, StudentImportRowError{Row: row.rowNumber, Message: "Belt rank does not exist."})
			continue
		}

		recordID := row.id
		createdAt := now
		if row.studentCode != nil && *row.studentCode != "" {
			existingByCode, err := s.store.FindStudentByCode(ctx, tx, *row.studentCode, "")
			if err != nil {
				return ImportStudentsResponse{}, err
			}
			if existingByCode != nil {
				recordID = existingByCode.RecordID

				var existingStudent StudentRecord
				if err := json.Unmarshal(existingByCode.Payload, &existingStudent); err != nil {
					return ImportStudentsResponse{}, err
				}
				createdAt = existingStudent.CreatedAt
			}
		}

		studentCode := row.studentCode
		if studentCode == nil || *studentCode == "" {
			generatedCode, err := s.store.NextStudentCode(ctx, tx)
			if err != nil {
				return ImportStudentsResponse{}, err
			}
			studentCode = &generatedCode
		}

		record := StudentRecord{
			BaseRecord: BaseRecord{
				ID:             recordID,
				CreatedAt:      createdAt,
				UpdatedAt:      now,
				LastModifiedAt: now,
				SyncStatus:     "synced",
			},
			StudentCode: studentCode,
			FullName:    row.fullName,
			DateOfBirth: row.dateOfBirth,
			Gender:      row.gender,
			Phone:       row.phone,
			Email:       row.email,
			Address:     row.address,
			ClubID:      club.ID,
			GroupID:     groupID,
			BeltRankID:  beltRankRecord.RecordID,
			JoinedAt:    row.joinedAt,
			Status:      row.status,
			Notes:       row.notes,
		}

		payload, err := json.Marshal(record)
		if err != nil {
			return ImportStudentsResponse{}, err
		}

		storedRecord := StoredRecord{
			EntityName:       EntityStudents,
			RecordID:         recordID,
			Payload:          payload,
			LastModifiedAt:   now,
			ServerModifiedAt: now,
		}

		if err := s.store.UpsertRecord(ctx, tx, storedRecord); err != nil {
			rowErrors = append(rowErrors, StudentImportRowError{Row: row.rowNumber, Message: err.Error()})
			continue
		}

		clubWeekdays, err := s.store.ListClubScheduleWeekdays(ctx, tx, club.ID)
		if err != nil {
			return ImportStudentsResponse{}, err
		}
		if row.scheduleMode == "custom" {
			clubWeekdaySet := make(map[string]struct{}, len(clubWeekdays))
			for _, weekday := range clubWeekdays {
				clubWeekdaySet[weekday] = struct{}{}
			}
			for _, weekday := range row.scheduleDays {
				if _, exists := clubWeekdaySet[weekday]; !exists {
					rowErrors = append(rowErrors, StudentImportRowError{
						Row:     row.rowNumber,
						Message: "Custom schedule must stay within the selected club schedule.",
					})
					goto nextRow
				}
			}
		}

		if err := s.store.ReplaceStudentSchedule(ctx, tx, recordID, row.scheduleMode, row.scheduleDays, now); err != nil {
			return ImportStudentsResponse{}, err
		}

		importedCount += 1
		changedEntityNames = append(changedEntityNames, EntityStudents, EntityStudentScheduleProfiles)
		changedIDs = append(changedIDs, recordID, recordID)
		for _, weekday := range row.scheduleDays {
			changedEntityNames = append(changedEntityNames, EntityStudentSchedules)
			changedIDs = append(changedIDs, fmt.Sprintf("%s:%s", recordID, weekday))
		}
	nextRow:
	}

	if err := tx.Commit(ctx); err != nil {
		return ImportStudentsResponse{}, err
	}

	if len(changedIDs) > 0 {
		s.hub.BroadcastChange(changedEntityNames, changedIDs)
	}

	return ImportStudentsResponse{
		ImportedCount: importedCount,
		Errors:        rowErrors,
	}, nil
}

func resolveStudentImportColumns(headerRow []string) (studentImportColumns, error) {
	columns := studentImportColumns{
		studentCodeIndex:  -1,
		fullNameIndex:     -1,
		dateOfBirthIndex:  -1,
		genderIndex:       -1,
		phoneIndex:        -1,
		emailIndex:        -1,
		addressIndex:      -1,
		clubIndex:         -1,
		groupIndex:        -1,
		beltRankIndex:     -1,
		joinedAtIndex:     -1,
		statusIndex:       -1,
		notesIndex:        -1,
		scheduleModeIndex: -1,
		scheduleDaysIndex: -1,
	}

	for index, value := range headerRow {
		switch normalizeImportHeader(value) {
		case "studentcode", "code":
			columns.studentCodeIndex = index
		case "fullname", "name":
			columns.fullNameIndex = index
		case "dateofbirth", "dob":
			columns.dateOfBirthIndex = index
		case "gender":
			columns.genderIndex = index
		case "phone":
			columns.phoneIndex = index
		case "email":
			columns.emailIndex = index
		case "address":
			columns.addressIndex = index
		case "club", "clubname":
			columns.clubIndex = index
		case "group", "groupname":
			columns.groupIndex = index
		case "beltrank", "beltrankname":
			columns.beltRankIndex = index
		case "joinedat", "joindate":
			columns.joinedAtIndex = index
		case "status":
			columns.statusIndex = index
		case "notes":
			columns.notesIndex = index
		case "schedulemode":
			columns.scheduleModeIndex = index
		case "scheduledays", "trainingdays":
			columns.scheduleDaysIndex = index
		}
	}

	if columns.fullNameIndex < 0 || columns.clubIndex < 0 || columns.beltRankIndex < 0 {
		return studentImportColumns{}, errors.New("excel header must include columns: fullName, club, beltRank")
	}

	return columns, nil
}

func parseStudentImportRows(rows [][]string, columns studentImportColumns) ([]studentImportRow, []StudentImportRowError) {
	parsedRows := make([]studentImportRow, 0, len(rows))
	parseErrors := make([]StudentImportRowError, 0)
	seenStudentCodes := make(map[string]int)

	for index, row := range rows {
		rowNumber := index + 2
		if isEmptyImportRow(row) {
			continue
		}

		fullName := strings.TrimSpace(getImportCell(row, columns.fullNameIndex))
		if fullName == "" {
			parseErrors = append(parseErrors, StudentImportRowError{Row: rowNumber, Message: "Full name is required."})
			continue
		}

		clubName := strings.TrimSpace(getImportCell(row, columns.clubIndex))
		if clubName == "" {
			parseErrors = append(parseErrors, StudentImportRowError{Row: rowNumber, Message: "Club is required."})
			continue
		}

		beltRankName := strings.TrimSpace(getImportCell(row, columns.beltRankIndex))
		if beltRankName == "" {
			parseErrors = append(parseErrors, StudentImportRowError{Row: rowNumber, Message: "Belt rank is required."})
			continue
		}

		studentCodeValue := strings.TrimSpace(getImportCell(row, columns.studentCodeIndex))
		if studentCodeValue != "" {
			if !studentCodePattern.MatchString(studentCodeValue) {
				parseErrors = append(parseErrors, StudentImportRowError{Row: rowNumber, Message: "Student code format is invalid."})
				continue
			}
			if previousRow, exists := seenStudentCodes[studentCodeValue]; exists {
				parseErrors = append(parseErrors, StudentImportRowError{
					Row:     rowNumber,
					Message: fmt.Sprintf("Duplicate student code in file (same as row %d).", previousRow),
				})
				continue
			}
		}

		dateOfBirth, err := parseImportDate(getImportCell(row, columns.dateOfBirthIndex), "Date of birth")
		if err != nil {
			parseErrors = append(parseErrors, StudentImportRowError{Row: rowNumber, Message: err.Error()})
			continue
		}

		joinedAt, err := parseImportDate(getImportCell(row, columns.joinedAtIndex), "Joined date")
		if err != nil {
			parseErrors = append(parseErrors, StudentImportRowError{Row: rowNumber, Message: err.Error()})
			continue
		}

		if dateOfBirth != nil && joinedAt != nil && *joinedAt < *dateOfBirth {
			parseErrors = append(parseErrors, StudentImportRowError{Row: rowNumber, Message: "Joined date cannot be earlier than date of birth."})
			continue
		}

		gender, err := parseImportGender(getImportCell(row, columns.genderIndex))
		if err != nil {
			parseErrors = append(parseErrors, StudentImportRowError{Row: rowNumber, Message: err.Error()})
			continue
		}

		status, err := parseImportStudentStatus(getImportCell(row, columns.statusIndex))
		if err != nil {
			parseErrors = append(parseErrors, StudentImportRowError{Row: rowNumber, Message: err.Error()})
			continue
		}

		email := strings.TrimSpace(getImportCell(row, columns.emailIndex))
		if email != "" && !emailPattern.MatchString(email) {
			parseErrors = append(parseErrors, StudentImportRowError{Row: rowNumber, Message: "Email format is invalid."})
			continue
		}

		phone := strings.TrimSpace(getImportCell(row, columns.phoneIndex))
		if phone != "" && !phonePattern.MatchString(phone) {
			parseErrors = append(parseErrors, StudentImportRowError{Row: rowNumber, Message: "Phone number format is invalid."})
			continue
		}

		groupName := strings.TrimSpace(getImportCell(row, columns.groupIndex))
		scheduleMode, err := parseImportScheduleMode(getImportCell(row, columns.scheduleModeIndex))
		if err != nil {
			parseErrors = append(parseErrors, StudentImportRowError{Row: rowNumber, Message: err.Error()})
			continue
		}
		scheduleDays, err := parseImportScheduleDays(getImportCell(row, columns.scheduleDaysIndex))
		if err != nil {
			parseErrors = append(parseErrors, StudentImportRowError{Row: rowNumber, Message: err.Error()})
			continue
		}
		if scheduleMode == "custom" && len(scheduleDays) == 0 {
			parseErrors = append(parseErrors, StudentImportRowError{Row: rowNumber, Message: "Schedule days are required when schedule mode is custom."})
			continue
		}
		if studentCodeValue != "" {
			seenStudentCodes[studentCodeValue] = rowNumber
		}

		parsedRows = append(parsedRows, studentImportRow{
			rowNumber:    rowNumber,
			id:           generateImportedStudentID(),
			studentCode:  stringPtrOrNil(studentCodeValue),
			fullName:     fullName,
			dateOfBirth:  dateOfBirth,
			gender:       gender,
			phone:        stringPtrOrNil(phone),
			email:        stringPtrOrNil(email),
			address:      stringPtrOrNil(strings.TrimSpace(getImportCell(row, columns.addressIndex))),
			clubName:     clubName,
			groupName:    stringPtrOrNil(groupName),
			beltRankName: beltRankName,
			joinedAt:     joinedAt,
			status:       status,
			notes:        stringPtrOrNil(strings.TrimSpace(getImportCell(row, columns.notesIndex))),
			scheduleMode: scheduleMode,
			scheduleDays: scheduleDays,
		})
	}

	return parsedRows, parseErrors
}

func parseImportDate(value string, fieldLabel string) (*string, error) {
	rawValue := strings.TrimSpace(value)
	if rawValue == "" {
		return nil, nil
	}

	layouts := []string{"2006-01-02", "2006/01/02"}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, rawValue)
		if err == nil {
			formatted := parsed.Format("2006-01-02")
			return &formatted, nil
		}
	}

	return nil, fmt.Errorf("%s must be in YYYY-MM-DD format.", fieldLabel)
}

func parseImportGender(value string) (*string, error) {
	rawValue := strings.TrimSpace(value)
	if rawValue == "" {
		return nil, nil
	}

	normalized := strings.ToLower(removeAccents(rawValue))
	switch normalized {
	case "male", "m", "nam":
		value := "male"
		return &value, nil
	case "female", "f", "nu":
		value := "female"
		return &value, nil
	default:
		return nil, errors.New("Gender must be one of: male, female.")
	}
}

func parseImportStudentStatus(value string) (string, error) {
	rawValue := strings.TrimSpace(value)
	if rawValue == "" {
		return "active", nil
	}

	normalized := strings.ToLower(removeAccents(rawValue))
	switch normalized {
	case "active", "dang hoc":
		return "active", nil
	case "inactive", "nghi hoc":
		return "inactive", nil
	case "suspended", "tam dung":
		return "suspended", nil
	default:
		return "", errors.New("Status must be one of: active, inactive, suspended.")
	}
}

func parseImportScheduleMode(value string) (string, error) {
	rawValue := strings.TrimSpace(value)
	if rawValue == "" {
		return "inherit", nil
	}

	switch strings.ToLower(removeAccents(rawValue)) {
	case "inherit", "club", "default":
		return "inherit", nil
	case "custom":
		return "custom", nil
	default:
		return "", errors.New("Schedule mode must be one of: inherit, custom.")
	}
}

func parseImportScheduleDays(value string) ([]string, error) {
	rawValue := strings.TrimSpace(value)
	if rawValue == "" {
		return nil, nil
	}

	parts := strings.FieldsFunc(rawValue, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == '/'
	})
	weekdays := make([]string, 0, len(parts))
	seen := make(map[string]struct{})
	for _, part := range parts {
		normalized := strings.ToLower(strings.TrimSpace(part))
		switch normalized {
		case "mon", "monday", "thu2", "t2":
			normalized = "mon"
		case "tue", "tuesday", "thu3", "t3":
			normalized = "tue"
		case "wed", "wednesday", "thu4", "t4":
			normalized = "wed"
		case "thu", "thursday", "thu5", "t5":
			normalized = "thu"
		case "fri", "friday", "thu6", "t6":
			normalized = "fri"
		case "sat", "saturday", "thu7", "t7":
			normalized = "sat"
		case "sun", "sunday", "cn", "chu nhat":
			normalized = "sun"
		default:
			return nil, errors.New("Schedule days must use weekdays like mon, wed, fri.")
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		weekdays = append(weekdays, normalized)
	}

	return weekdays, nil
}

func generateImportedStudentID() string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("student-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("student-%s", hex.EncodeToString(buffer))
}

func (s *Service) GenerateStudentImportTemplate() ([]byte, error) {
	workbook := excelize.NewFile()
	defer workbook.Close()

	sheetName := workbook.GetSheetName(workbook.GetActiveSheetIndex())
	headers := []string{
		"fullName",
		"club",
		"group",
		"beltRank",
		"scheduleMode",
		"scheduleDays",
		"studentCode",
		"dateOfBirth",
		"gender",
		"phone",
		"email",
		"address",
		"joinedAt",
		"status",
		"notes",
	}
	sampleRows := [][]string{
		{
			"Nguyen Van A",
			"Phong Quyen Quan",
			"Beginner",
			"White Belt",
			"inherit",
			"",
			"",
			"2012-05-10",
			"male",
			"0901234567",
			"student.a@example.com",
			"Ho Chi Minh City",
			"2026-03-01",
			"active",
			"Sample row",
		},
		{
			"Tran Thi B",
			"Phong Quyen Quan",
			"Advanced",
			"Yellow Belt",
			"custom",
			"wed,fri",
			"PQQ-000123",
			"2010-08-20",
			"female",
			"0912345678",
			"",
			"Thu Duc",
			"2025-11-20",
			"active",
			"",
		},
	}

	for index, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(index+1, 1)
		if err := workbook.SetCellValue(sheetName, cell, header); err != nil {
			return nil, err
		}
	}

	for rowIndex, row := range sampleRows {
		for columnIndex, value := range row {
			cell, _ := excelize.CoordinatesToCellName(columnIndex+1, rowIndex+2)
			if err := workbook.SetCellValue(sheetName, cell, value); err != nil {
				return nil, err
			}
		}
	}

	headerStyle, err := workbook.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "#0F172A"},
		Fill:      excelize.Fill{Type: "pattern", Color: []string{"#E2E8F0"}, Pattern: 1},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	if err != nil {
		return nil, err
	}

	if err := workbook.SetCellStyle(sheetName, "A1", "O1", headerStyle); err != nil {
		return nil, err
	}

	if err := workbook.SetPanes(sheetName, &excelize.Panes{
		Freeze:      true,
		Split:       false,
		XSplit:      0,
		YSplit:      1,
		TopLeftCell: "A2",
		ActivePane:  "bottomLeft",
	}); err != nil {
		return nil, err
	}

	for _, column := range []string{"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M", "N", "O"} {
		if err := workbook.SetColWidth(sheetName, column, column, 18); err != nil {
			return nil, err
		}
	}

	buffer := bytes.NewBuffer(nil)
	if err := workbook.Write(buffer); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}
