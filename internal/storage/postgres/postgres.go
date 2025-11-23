package postgres

import (
	"database/sql"
	"fmt"
	"log"

	_ "github.com/lib/pq" //
	"main.go/internal/models"
)

type Storage struct {
	db *sql.DB
}

// New - инициализация БД, создание таблиц, если их нет
func New(storagePath string) (*Storage, error) {
	const op = "storage.postgres.New"

	db, err := sql.Open("postgres", storagePath)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	queries := []string{
		`CREATE TABLE IF NOT EXISTS teams (
            team_name VARCHAR(255) NOT NULL PRIMARY KEY
        );`,
		`CREATE TABLE IF NOT EXISTS users (
            user_id VARCHAR(255) NOT NULL PRIMARY KEY,
            user_name VARCHAR(255) NOT NULL,
            team_name VARCHAR(255) REFERENCES teams(team_name),
            is_active BOOLEAN NOT NULL DEFAULT true
        );`,
		`CREATE TABLE IF NOT EXISTS pull_requests (
            pull_request_id VARCHAR(255) NOT NULL PRIMARY KEY,
            pull_request_name VARCHAR(255) NOT NULL,
            author_id VARCHAR(255) REFERENCES users(user_id),
            status VARCHAR(10) NOT NULL CHECK (status IN ('OPEN','MERGED')),
            created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
            merged_at TIMESTAMPTZ
        );`,
		`CREATE TABLE IF NOT EXISTS pull_requests_reviewers (
            pull_request_id VARCHAR(255) REFERENCES pull_requests(pull_request_id),
            user_id VARCHAR(255) REFERENCES users(user_id),
            PRIMARY KEY (pull_request_id, user_id)
        );`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
	}

	return &Storage{db: db}, nil
}

// SaveTeam - сохранение команды с участниками, принимает структуру team
func (s *Storage) SaveTeam(team models.Team) error {

	const op = "storage.postgres.SaveTeam"
	stmt, err := s.db.Prepare("INSERT INTO teams(team_name) VALUES($1)")
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	defer stmt.Close()
	_, err = stmt.Exec(team.TeamName)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	memberStmt, err := s.db.Prepare("INSERT INTO users(user_id, user_name, team_name, is_active) VALUES($1,$2,$3,$4)")
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	defer memberStmt.Close()

	for i := range team.Members {
		_, err = memberStmt.Exec(
			team.Members[i].UserID,
			team.Members[i].UserName,
			team.TeamName,
			team.Members[i].IsActive,
		)
		if err != nil {
			return fmt.Errorf("%s: %w", op, err)
		}
	}

	return nil
}

// GetTeamMembers - получить членов команды
func (s *Storage) GetTeamMembers(teamName string) ([]models.TeamMember, error) {
	const op = "storage.postgres.GetTeamMembers"

	rows, err := s.db.Query(`
		SELECT user_id, user_name, is_active
		FROM users
		WHERE team_name = $1
		ORDER BY user_id
	`, teamName)

	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	var members []models.TeamMember
	for rows.Next() {
		var member models.TeamMember
		if err := rows.Scan(&member.UserID, &member.UserName, &member.IsActive); err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		members = append(members, member)
	}

	return members, rows.Err()
}

// TeamExists - проверить, существует ли команда
func (s *Storage) TeamExists(teamName string) (bool, error) {
	const op = "storage.postgres.TeamExists"

	var exists bool
	err := s.db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM teams WHERE team_name = $1)
	`, teamName).Scan(&exists)

	if err != nil {
		return false, fmt.Errorf("%s: %w", op, err)
	}

	return exists, nil
}

// SaveTeamWithUpdate - создать команду или обновить членов
func (s *Storage) SaveTeamWithUpdate(team models.Team) (bool, error) {
	const op = "storage.postgres.SaveTeamWithUpdate"

	tx, err := s.db.Begin()
	if err != nil {
		return false, fmt.Errorf("%s: failed to begin transaction: %w", op, err)
	}
	defer tx.Rollback()

	// Проверяем, существует ли команда
	var exists bool
	err = tx.QueryRow(`SELECT EXISTS(SELECT 1 FROM teams WHERE team_name = $1)`, team.TeamName).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("%s: %w", op, err)
	}

	// Если команда не существует, создаём её
	if !exists {
		_, err = tx.Exec(`INSERT INTO teams (team_name) VALUES ($1)`, team.TeamName)
		if err != nil {
			return false, fmt.Errorf("%s: failed to create team: %w", op, err)
		}
	}

	// Получаем текущих членов команды
	rows, err := tx.Query(`
		SELECT user_id FROM users WHERE team_name = $1
	`, team.TeamName)
	if err != nil {
		return false, fmt.Errorf("%s: failed to get current members: %w", op, err)
	}
	defer rows.Close()

	existingMembers := make(map[string]bool)
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return false, fmt.Errorf("%s: %w", op, err)
		}
		existingMembers[userID] = true
	}

	// Проверяем, есть ли новые члены
	hasNewMembers := false
	for _, member := range team.Members {
		if !existingMembers[member.UserID] {
			hasNewMembers = true
			break
		}
	}

	// Если нет новых членов и команда уже существует → ошибка
	if exists && !hasNewMembers {
		return false, fmt.Errorf("%s: team already exists with same members", op)
	}

	// Добавляем только новых членов
	for _, member := range team.Members {
		if !existingMembers[member.UserID] {
			_, err = tx.Exec(`
				INSERT INTO users (user_id, user_name, team_name, is_active)
				VALUES ($1, $2, $3, $4)
				ON CONFLICT (user_id) DO UPDATE SET
					user_name = $2,
					team_name = $3,
					is_active = $4
			`, member.UserID, member.UserName, team.TeamName, member.IsActive)

			if err != nil {
				return false, fmt.Errorf("%s: failed to add member: %w", op, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("%s: failed to commit: %w", op, err)
	}

	return !exists, nil // возвращаем true если это была первая создание команды
}

// SetUserActive - меняет флаг активности пользователя по id
func (s *Storage) SetUserActive(userID string, isActive bool) (*models.User, error) {
	const op = "storage.postgres.SetUserActive"

	log.Printf("%s: updating user %s to is_active=%v", op, userID, isActive)

	var user models.User

	err := s.db.QueryRow(`
        UPDATE users 
        SET is_active = $1 
        WHERE user_id = $2
        RETURNING user_id, user_name, team_name, is_active
    `, isActive, userID).Scan(
		&user.UserID,
		&user.UserName,
		&user.TeamName,
		&user.IsActive,
	)

	if err == sql.ErrNoRows {
		log.Printf("%s: user %s not found", op, userID)
		return nil, fmt.Errorf("%s: user not found", op)
	}

	if err != nil {
		log.Printf("%s: error updating user: %v", op, err)
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	log.Printf("%s: user updated successfully: %+v", op, user)

	return &user, nil
}

// CheckAuthorExist - функция проверяет, существует ли автор
func (s *Storage) CheckAuthorExist(authorID string) error {
	const op = "storage.postgres.CheckAuthorExist"
	var userID string
	err := s.db.QueryRow(`SELECT user_id FROM users WHERE user_id = $1`, authorID).Scan(&userID)

	if err == sql.ErrNoRows {
		return fmt.Errorf("%s: author are not found", op)
	}

	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	return nil
}

// CreatePullRequest - функция создает pull request и назначает ревьюеров
func (s *Storage) CreatePullRequest(pr models.PullRequest) error {
	const op = "storage.postgres.CreatePullRequest"

	// 1. Создаем PR в таблице pull_requests
	_, err := s.db.Exec(`
		INSERT INTO pull_requests(
			pull_request_id,
			pull_request_name,
			author_id,
			status
		)
		VALUES ($1, $2, $3, 'OPEN')
	`, pr.PullRequestID, pr.PullRequestName, pr.AuthorID)

	if err != nil {
		return fmt.Errorf("%s: failed to create PR: %w", op, err)
	}

	// 2. Добавляем ревьюеров в таблицу pull_requests_reviewers
	reviewerStmt, err := s.db.Prepare(`
		INSERT INTO pull_requests_reviewers(pull_request_id, user_id)
		VALUES ($1, $2)
	`)
	if err != nil {
		return fmt.Errorf("%s: failed to prepare reviewer statement: %w", op, err)
	}
	defer reviewerStmt.Close()

	// 3. Вставляем каждого ревьювера отдельной строкой
	for _, reviewerID := range pr.AssignedReviewers {
		_, err := reviewerStmt.Exec(pr.PullRequestID, reviewerID)
		if err != nil {
			return fmt.Errorf("%s: failed to add reviewer %s: %w", op, reviewerID, err)
		}
	}

	return nil
}

// Функция находит всех участников команды автора и возвращает список активных
func (s *Storage) GetActiveTeamMembers(authorID string) ([]string, error) {
	const op = "storage.postgres.GetActiveTeamMember"
	//	2.Получаем всех активных участников команды (кроме автора)
	rows, err := s.db.Query(`
    SELECT user_id
    FROM users
    WHERE team_name = (
        SELECT team_name
        FROM users
        WHERE user_id = $1)
    AND is_active = true
    AND user_id != $1
	`, authorID)

	if err != nil {
		return nil, fmt.Errorf("%s: failed to get reviewrs: %w", op, err)
	}
	defer rows.Close()

	var reviewrs []string
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		reviewrs = append(reviewrs, userID)
	}
	return reviewrs, nil
}

// GetPullRequestReviewers - получить список ревьюеров PR
func (s *Storage) GetPullRequestReviewers(pullRequestID string) ([]string, error) {
	const op = "storage.postgres.GetPullRequestReviewers"

	rows, err := s.db.Query(`
		SELECT user_id
		FROM pull_requests_reviewers
		WHERE pull_request_id = $1
	`, pullRequestID)

	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	var reviewers []string
	for rows.Next() {
		var reviewerID string
		if err := rows.Scan(&reviewerID); err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		reviewers = append(reviewers, reviewerID)
	}

	return reviewers, rows.Err()
}

// MergePullRequest - пометить PR как MERGED (идемпотентная операция)
func (s *Storage) MergePullRequest(pullRequestID string) (*models.PullRequest, error) {
	const op = "storage.postgres.MergePullRequest"

	var pr models.PullRequest

	// UPDATE с RETURNING - обновляем и сразу получаем данные
	err := s.db.QueryRow(`
		UPDATE pull_requests
		SET status = 'MERGED', merged_at = NOW()
		WHERE pull_request_id = $1
		RETURNING pull_request_id, pull_request_name, author_id, status, created_at, merged_at
	`, pullRequestID).Scan(
		&pr.PullRequestID,
		&pr.PullRequestName,
		&pr.AuthorID,
		&pr.Status,
		&pr.CreatedAt,
		&pr.MergedAt,
	)

	// Если PR не найден
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%s: pull request not found", op)
	}

	// Другие ошибки БД
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	// Получаем assigned_reviewers через отдельную функцию ✅
	reviewers, err := s.GetPullRequestReviewers(pullRequestID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	pr.AssignedReviewers = reviewers

	return &pr, nil
}

// GetPullRequestByID - получить PR по ID с проверкой статуса
func (s *Storage) GetPullRequestByID(pullRequestID string) (*models.PullRequest, error) {
	const op = "storage.postgres.GetPullRequestByID"

	var pr models.PullRequest

	// Получаем основную информацию о PR
	err := s.db.QueryRow(`
		SELECT pull_request_id, pull_request_name, author_id, status, created_at, merged_at
		FROM pull_requests
		WHERE pull_request_id = $1
	`, pullRequestID).Scan(
		&pr.PullRequestID,
		&pr.PullRequestName,
		&pr.AuthorID,
		&pr.Status,
		&pr.CreatedAt,
		&pr.MergedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("%s: pull request not found", op)
	}

	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	// Получаем список ревьюеров
	reviewers, err := s.GetPullRequestReviewers(pullRequestID)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}

	pr.AssignedReviewers = reviewers
	return &pr, nil
}

// ReassignReviewer - заменить ревьювера на другого в PR
func (s *Storage) ReassignReviewer(pullRequestID, oldReviewerID, newReviewerID string) error {
	const op = "storage.postgres.ReassignReviewer"

	// Используем транзакцию для атомарности
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("%s: failed to begin transaction: %w", op, err)
	}
	defer tx.Rollback()

	// 1. Удаляем старого ревьювера
	_, err = tx.Exec(`
		DELETE FROM pull_requests_reviewers
		WHERE pull_request_id = $1 AND user_id = $2
	`, pullRequestID, oldReviewerID)

	if err != nil {
		return fmt.Errorf("%s: failed to delete old reviewer: %w", op, err)
	}

	// 2. Добавляем нового ревьювера
	_, err = tx.Exec(`
		INSERT INTO pull_requests_reviewers (pull_request_id, user_id)
		VALUES ($1, $2)
	`, pullRequestID, newReviewerID)

	if err != nil {
		return fmt.Errorf("%s: failed to insert new reviewer: %w", op, err)
	}

	// Коммитим транзакцию
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%s: failed to commit transaction: %w", op, err)
	}

	return nil
}

// IsReviewerAssigned - проверить, назначен ли пользователь ревьювером на PR
func (s *Storage) IsReviewerAssigned(pullRequestID, userID string) (bool, error) {
	const op = "storage.postgres.IsReviewerAssigned"

	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*)
		FROM pull_requests_reviewers
		WHERE pull_request_id = $1 AND user_id = $2
	`, pullRequestID, userID).Scan(&count)

	if err != nil {
		return false, fmt.Errorf("%s: %w", op, err)
	}

	return count > 0, nil
}

// GetUserAssignedPullRequests - получить все PR'ы, где пользователь назначен ревьювером
func (s *Storage) GetUserAssignedPullRequests(userID string) ([]models.PullRequestShort, error) {
	const op = "storage.postgres.GetUserAssignedPullRequests"

	rows, err := s.db.Query(`
		SELECT 
			pr.pull_request_id,
			pr.pull_request_name,
			pr.author_id,
			pr.status
		FROM pull_requests pr
		INNER JOIN pull_requests_reviewers prr ON pr.pull_request_id = prr.pull_request_id
		WHERE prr.user_id = $1
		ORDER BY pr.created_at DESC
	`, userID)

	if err != nil {
		return nil, fmt.Errorf("%s: %w", op, err)
	}
	defer rows.Close()

	var pullRequests []models.PullRequestShort
	for rows.Next() {
		var pr models.PullRequestShort
		if err := rows.Scan(&pr.PullRequestID, &pr.PullRequestName, &pr.AuthorID, &pr.Status); err != nil {
			return nil, fmt.Errorf("%s: %w", op, err)
		}
		pullRequests = append(pullRequests, pr)
	}

	return pullRequests, rows.Err()
}

// CheckUserExists - проверить, существует ли пользователь
func (s *Storage) CheckUserExists(userID string) error {
	const op = "storage.postgres.CheckUserExists"

	var exists bool
	err := s.db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM users WHERE user_id = $1)
	`, userID).Scan(&exists)

	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}

	if !exists {
		return fmt.Errorf("%s: user not found", op)
	}

	return nil
}
