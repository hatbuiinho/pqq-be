package db

import "time"

type Club struct {
	ID             string
	Code           *string
	Name           string
	Phone          *string
	Email          *string
	Address        *string
	Notes          *string
	IsActive       bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastModifiedAt time.Time
	DeletedAt      *time.Time
}

type ClubGroup struct {
	ID             string
	ClubID         string
	Name           string
	Description    *string
	IsActive       bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastModifiedAt time.Time
	DeletedAt      *time.Time
}

type ClubSchedule struct {
	ID             string
	ClubID         string
	Weekday        string
	IsActive       bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastModifiedAt time.Time
	DeletedAt      *time.Time
}

type BeltRank struct {
	ID             string
	Name           string
	RankOrder      int32
	Description    *string
	IsActive       bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastModifiedAt time.Time
	DeletedAt      *time.Time
}

type Student struct {
	ID             string
	StudentCode    *string
	FullName       string
	DateOfBirth    *time.Time
	Gender         *string
	Phone          *string
	Email          *string
	Address        *string
	ClubID         string
	GroupID        *string
	BeltRankID     string
	JoinedAt       *time.Time
	Status         string
	Notes          *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastModifiedAt time.Time
	DeletedAt      *time.Time
}

type StudentScheduleProfile struct {
	ID             string
	StudentID      string
	Mode           string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastModifiedAt time.Time
	DeletedAt      *time.Time
}

type StudentSchedule struct {
	ID             string
	StudentID      string
	Weekday        string
	IsActive       bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastModifiedAt time.Time
	DeletedAt      *time.Time
}

type AttendanceSession struct {
	ID             string
	ClubID         string
	SessionDate    time.Time
	Status         string
	Notes          *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	LastModifiedAt time.Time
	DeletedAt      *time.Time
}

type AttendanceRecord struct {
	ID               string
	SessionID        string
	StudentID        string
	AttendanceStatus string
	CheckInAt        *time.Time
	Notes            *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
	LastModifiedAt   time.Time
	DeletedAt        *time.Time
}

type FindActiveClubByCodeParams struct {
	ExcludeID string
	Code      string
}

type FindActiveClubScheduleByWeekdayParams struct {
	ExcludeID string
	ClubID    string
	Weekday   string
}

type FindActiveBeltRankByOrderParams struct {
	ExcludeID string
	RankOrder int32
}

type FindActiveStudentByCodeParams struct {
	ExcludeID   string
	StudentCode string
}

type FindActiveStudentScheduleByWeekdayParams struct {
	ExcludeID string
	StudentID string
	Weekday   string
}

type FindActiveAttendanceRecordBySessionAndStudentParams struct {
	ExcludeID string
	SessionID string
	StudentID string
}
