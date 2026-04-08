package sync

import "encoding/json"

type EntityName string
type Operation string
type AttendanceActionType string

const (
	EntityClubs                   EntityName = "clubs"
	EntityClubGroups              EntityName = "club_groups"
	EntityClubSchedules           EntityName = "club_schedules"
	EntityBeltRanks               EntityName = "belt_ranks"
	EntityStudents                EntityName = "students"
	EntityStudentMessages         EntityName = "student_messages"
	EntityStudentScheduleProfiles EntityName = "student_schedule_profiles"
	EntityStudentSchedules        EntityName = "student_schedules"
	EntityAttendanceSessions      EntityName = "attendance_sessions"
	EntityAttendanceRecords       EntityName = "attendance_records"

	OperationUpsert Operation = "upsert"
	OperationDelete Operation = "delete"

	AttendanceActionCreateSession    AttendanceActionType = "create_session"
	AttendanceActionSetRecordStatus  AttendanceActionType = "set_record_status"
	AttendanceActionSetRecordNote    AttendanceActionType = "set_record_note"
	AttendanceActionSetSessionNote   AttendanceActionType = "set_session_note"
	AttendanceActionSetSessionStatus AttendanceActionType = "set_session_status"
	AttendanceActionDeleteSession    AttendanceActionType = "delete_session"
)

type PushRequest struct {
	DeviceID  string         `json:"deviceId"`
	Mutations []SyncMutation `json:"mutations"`
}

type SyncMutation struct {
	MutationID       string          `json:"mutationId"`
	EntityName       EntityName      `json:"entityName"`
	Operation        Operation       `json:"operation"`
	RecordID         string          `json:"recordId"`
	Record           json.RawMessage `json:"record"`
	ClientModifiedAt string          `json:"clientModifiedAt"`
}

type AppliedRecord struct {
	EntityName       EntityName      `json:"entityName"`
	Record           json.RawMessage `json:"record"`
	ServerModifiedAt string          `json:"serverModifiedAt"`
}

type Conflict struct {
	MutationID   string          `json:"mutationId"`
	EntityName   EntityName      `json:"entityName"`
	RecordID     string          `json:"recordId"`
	Reason       string          `json:"reason"`
	Message      string          `json:"message"`
	ServerRecord json.RawMessage `json:"serverRecord,omitempty"`
}

type PushResponse struct {
	ServerTime string          `json:"serverTime"`
	Applied    []AppliedRecord `json:"applied"`
	Conflicts  []Conflict      `json:"conflicts"`
}

type AttendanceActionPushRequest struct {
	DeviceID string                     `json:"deviceId"`
	Actions  []AttendanceActionMutation `json:"actions"`
}

type AttendanceActionMutation struct {
	ActionID         string               `json:"actionId"`
	ActionType       AttendanceActionType `json:"actionType"`
	ClubID           string               `json:"clubId"`
	SessionID        string               `json:"sessionId"`
	RecordID         *string              `json:"recordId,omitempty"`
	StudentID        *string              `json:"studentId,omitempty"`
	Payload          json.RawMessage      `json:"payload"`
	ClientOccurredAt string               `json:"clientOccurredAt"`
}

type AttendanceActionAppliedChange struct {
	EntityName       EntityName      `json:"entityName"`
	Record           json.RawMessage `json:"record"`
	ServerModifiedAt string          `json:"serverModifiedAt"`
}

type AttendanceActionError struct {
	ActionID      string               `json:"actionId"`
	ActionType    AttendanceActionType `json:"actionType"`
	Message       string               `json:"message"`
	RecordID      *string              `json:"recordId,omitempty"`
	SessionID     string               `json:"sessionId"`
	StudentID     *string              `json:"studentId,omitempty"`
	ServerSession json.RawMessage      `json:"serverSession,omitempty"`
	ServerRecord  json.RawMessage      `json:"serverRecord,omitempty"`
}

type AttendanceActionPushResponse struct {
	ServerTime       string                          `json:"serverTime"`
	AppliedActionIDs []string                        `json:"appliedActionIds"`
	Changes          []AttendanceActionAppliedChange `json:"changes"`
	Errors           []AttendanceActionError         `json:"errors"`
}

type PullRequest struct {
	DeviceID string
	Since    string
	Limit    int
}

type PullChange struct {
	EntityName       EntityName      `json:"entityName"`
	Record           json.RawMessage `json:"record"`
	ServerModifiedAt string          `json:"serverModifiedAt"`
}

type PullResponse struct {
	ServerTime string       `json:"serverTime"`
	NextSince  string       `json:"nextSince"`
	HasMore    bool         `json:"hasMore"`
	Changes    []PullChange `json:"changes"`
}

type RebaseResponse struct {
	ServerTime              string                         `json:"serverTime"`
	Clubs                   []ClubRecord                   `json:"clubs"`
	ClubGroups              []ClubGroupRecord              `json:"clubGroups"`
	ClubSchedules           []ClubScheduleRecord           `json:"clubSchedules"`
	BeltRanks               []BeltRankRecord               `json:"beltRanks"`
	Students                []StudentRecord                `json:"students"`
	StudentMessages         []StudentMessageRecord         `json:"studentMessages"`
	StudentScheduleProfiles []StudentScheduleProfileRecord `json:"studentScheduleProfiles"`
	StudentSchedules        []StudentScheduleRecord        `json:"studentSchedules"`
	AttendanceSessions      []AttendanceSessionRecord      `json:"attendanceSessions"`
	AttendanceRecords       []AttendanceRecord             `json:"attendanceRecords"`
}

type ClubImportRowError struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

type ImportClubsResponse struct {
	ImportedCount int                  `json:"importedCount"`
	Errors        []ClubImportRowError `json:"errors"`
}

type BeltRankImportRowError struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

type ImportBeltRanksResponse struct {
	ImportedCount int                      `json:"importedCount"`
	Errors        []BeltRankImportRowError `json:"errors"`
}

type StudentImportRowError struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

type ImportStudentsResponse struct {
	ImportedCount int                     `json:"importedCount"`
	Errors        []StudentImportRowError `json:"errors"`
}

type StudentPublicProfile struct {
	ID          string  `json:"id"`
	StudentCode *string `json:"studentCode,omitempty"`
	FullName    string  `json:"fullName"`
	DateOfBirth *string `json:"dateOfBirth,omitempty"`
	Gender      *string `json:"gender,omitempty"`
	Phone       *string `json:"phone,omitempty"`
	Email       *string `json:"email,omitempty"`
	Address     *string `json:"address,omitempty"`
	Status      string  `json:"status"`
	JoinedAt    *string `json:"joinedAt,omitempty"`
	Notes       *string `json:"notes,omitempty"`
	ClubID      string  `json:"clubId"`
	ClubName    string  `json:"clubName"`
	GroupID     *string `json:"groupId,omitempty"`
	GroupName   *string `json:"groupName,omitempty"`
	BeltRankID  string  `json:"beltRankId"`
	BeltRank    string  `json:"beltRankName"`
}

type RealtimeEvent struct {
	Type         string       `json:"type"`
	ConnectionID string       `json:"connectionId,omitempty"`
	ServerTime   string       `json:"serverTime"`
	EntityNames  []EntityName `json:"entityNames,omitempty"`
	RecordIDs    []string     `json:"recordIds,omitempty"`
}

type BaseRecord struct {
	ID             string  `json:"id"`
	CreatedAt      string  `json:"createdAt"`
	UpdatedAt      string  `json:"updatedAt"`
	LastModifiedAt string  `json:"lastModifiedAt"`
	SyncStatus     string  `json:"syncStatus"`
	DeletedAt      *string `json:"deletedAt,omitempty"`
}

type ClubRecord struct {
	BaseRecord
	Code     *string `json:"code,omitempty"`
	Name     string  `json:"name"`
	Phone    *string `json:"phone,omitempty"`
	Email    *string `json:"email,omitempty"`
	Address  *string `json:"address,omitempty"`
	Notes    *string `json:"notes,omitempty"`
	IsActive bool    `json:"isActive"`
}

type BeltRankRecord struct {
	BaseRecord
	Name        string  `json:"name"`
	Order       int     `json:"order"`
	Description *string `json:"description,omitempty"`
	IsActive    bool    `json:"isActive"`
}

type ClubGroupRecord struct {
	BaseRecord
	ClubID      string  `json:"clubId"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	IsActive    bool    `json:"isActive"`
}

type ClubScheduleRecord struct {
	BaseRecord
	ClubID   string `json:"clubId"`
	Weekday  string `json:"weekday"`
	IsActive bool   `json:"isActive"`
}

type StudentRecord struct {
	BaseRecord
	StudentCode *string `json:"studentCode,omitempty"`
	FullName    string  `json:"fullName"`
	DateOfBirth *string `json:"dateOfBirth,omitempty"`
	Gender      *string `json:"gender,omitempty"`
	Phone       *string `json:"phone,omitempty"`
	Email       *string `json:"email,omitempty"`
	Address     *string `json:"address,omitempty"`
	ClubID      string  `json:"clubId"`
	GroupID     *string `json:"groupId,omitempty"`
	BeltRankID  string  `json:"beltRankId"`
	JoinedAt    *string `json:"joinedAt,omitempty"`
	Status      string  `json:"status"`
	Notes       *string `json:"notes,omitempty"`
}

type StudentMessageRecord struct {
	BaseRecord
	StudentID             string  `json:"studentId"`
	ClubID                string  `json:"clubId"`
	MessageType           string  `json:"messageType"`
	Content               string  `json:"content"`
	AuthorUserID          *string `json:"authorUserId,omitempty"`
	AuthorName            string  `json:"authorName"`
	AttendanceSessionID   *string `json:"attendanceSessionId,omitempty"`
	AttendanceRecordID    *string `json:"attendanceRecordId,omitempty"`
	AttendanceSessionDate *string `json:"attendanceSessionDate,omitempty"`
	AttendanceStatus      *string `json:"attendanceStatus,omitempty"`
}

type StudentScheduleProfileRecord struct {
	BaseRecord
	StudentID string `json:"studentId"`
	Mode      string `json:"mode"`
}

type StudentScheduleRecord struct {
	BaseRecord
	StudentID string `json:"studentId"`
	Weekday   string `json:"weekday"`
	IsActive  bool   `json:"isActive"`
}

type AttendanceSessionRecord struct {
	BaseRecord
	ClubID      string  `json:"clubId"`
	SessionDate string  `json:"sessionDate"`
	Status      string  `json:"status"`
	Notes       *string `json:"notes,omitempty"`
}

type AttendanceRecord struct {
	BaseRecord
	SessionID        string  `json:"sessionId"`
	StudentID        string  `json:"studentId"`
	AttendanceStatus string  `json:"attendanceStatus"`
	CheckInAt        *string `json:"checkInAt,omitempty"`
	Notes            *string `json:"notes,omitempty"`
}

type StoredRecord struct {
	ChangeID         int64
	EntityName       EntityName
	RecordID         string
	Payload          []byte
	DeletedAt        *string
	LastModifiedAt   string
	ServerModifiedAt string
}
