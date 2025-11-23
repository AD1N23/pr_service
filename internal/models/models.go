package models

import "time"

type Team struct {
	TeamName string       `json:"teamname" db:"team_name"`
	Members  []TeamMember `json:"members"`
}

type TeamMember struct {
	UserID   string `json:"userid" db:"user_id"`
	UserName string `json:"username" db:"user_name"`
	IsActive bool   `json:"isactive" db:"is_active"`
}

type User struct {
	UserID   string `json:"userid" db:"user_id"`
	UserName string `json:"username" db:"user_name"`
	TeamName string `json:"teamname" db:"team_name"`
	IsActive bool   `json:"isactive" db:"is_active"`
}

type PullRequest struct {
	PullRequestID     string     `json:"pullrequestid" db:"pull_request_id"`
	PullRequestName   string     `json:"pullrequestname" db:"pull_request_name"`
	AuthorID          string     `json:"authorid" db:"author_id"`
	Status            string     `json:"status" db:"status"`
	AssignedReviewers []string   `json:"assignedreviewers"`
	CreatedAt         *time.Time `json:"createdAt" db:"created_at"`
	MergedAt          *time.Time `json:"mergedAt" db:"merged_at"`
}

type PullRequestShort struct {
	PullRequestID   string `json:"pullrequestid" db:"pull_request_id"`
	PullRequestName string `json:"pullrequestname" db:"pull_request_name"`
	AuthorID        string `json:"authorid" db:"author_id"`
	Status          string `json:"status" db:"status"`
}
