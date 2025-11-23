package reassign

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"main.go/internal/models"
)

// Request - структура запроса
type Request struct {
	PullRequestID string `json:"pull_request_id"`
	OldUserID     string `json:"old_reviewer_id"`
}

// Response - структура ответа
type Response struct {
	PullRequest models.PullRequest `json:"pr"`
	ReplacedBy  string             `json:"replaced_by"`
}

// PRReassignInterface - интерфейс для операции переназначения ревьювера
type PRReassignInterface interface {
	GetPullRequestByID(pullRequestID string) (*models.PullRequest, error)
	IsReviewerAssigned(pullRequestID, userID string) (bool, error)
	GetActiveTeamMembers(authorID string) ([]string, error)
	ReassignReviewer(pullRequestID, oldReviewerID, newReviewerID string) error
}

// New создаёт handler для POST /pullRequest/reassign
func New(log *slog.Logger, reassigner PRReassignInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "http-server.handlers.pr.reassign.New"

		// 1. Декодируем JSON из тела запроса
		var req Request
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			log.Error("failed to decode request", slog.String("op", op), slog.String("error", err.Error()))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    "INVALID_REQUEST",
					Message: "invalid request body",
				},
			})
			return
		}

		// 2. Валидация - проверяем, что оба поля заполнены
		if req.PullRequestID == "" || req.OldUserID == "" {
			log.Error("empty fields in request", slog.String("op", op))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    "INVALID_REQUEST",
					Message: "pull_request_id and old_reviewer_id are required",
				},
			})
			return
		}

		log.Info("reassigning reviewer",
			slog.String("op", op),
			slog.String("pr_id", req.PullRequestID),
			slog.String("old_reviewer", req.OldUserID))

		// 3. Получаем PR по ID
		pullRequest, err := reassigner.GetPullRequestByID(req.PullRequestID)
		if err != nil {
			log.Error("failed to get PR", slog.String("op", op), slog.String("error", err.Error()))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    "NOT_FOUND",
					Message: "pull request not found",
				},
			})
			return
		}

		// 4. Проверяем, что PR в статусе OPEN (не MERGED)
		if pullRequest.Status == "MERGED" {
			log.Error("cannot reassign on merged PR", slog.String("op", op), slog.String("pr_id", req.PullRequestID))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    "PR_MERGED",
					Message: "cannot reassign on merged PR",
				},
			})
			return
		}

		// 5. Проверяем, что старый ревьювер действительно назначен на этот PR
		isAssigned, err := reassigner.IsReviewerAssigned(req.PullRequestID, req.OldUserID)
		if err != nil {
			log.Error("failed to check reviewer assignment", slog.String("op", op), slog.String("error", err.Error()))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    "INTERNAL_ERROR",
					Message: "failed to check reviewer assignment",
				},
			})
			return
		}

		if !isAssigned {
			log.Error("reviewer not assigned", slog.String("op", op), slog.String("pr_id", req.PullRequestID), slog.String("old_reviewer", req.OldUserID))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    "NOT_ASSIGNED",
					Message: "reviewer is not assigned to this PR",
				},
			})
			return
		}

		// 6. Получаем всех активных членов команды автора
		teamMembers, err := reassigner.GetActiveTeamMembers(pullRequest.AuthorID)
		if err != nil {
			log.Error("failed to get team members", slog.String("op", op), slog.String("error", err.Error()))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    "INTERNAL_ERROR",
					Message: "failed to get team members",
				},
			})
			return
		}

		// 7. Находим нового ревьювера (первого из доступных, кто не старый ревьювер)
		var newReviewerID string
		for _, member := range teamMembers {
			// Не берем старого ревьювера и не берем текущих ревьюверов
			if member != req.OldUserID && !contains(pullRequest.AssignedReviewers, member) {
				newReviewerID = member
				break
			}
		}

		// 8. Если нет доступного кандидата
		if newReviewerID == "" {
			log.Error("no available replacement candidate", slog.String("op", op), slog.String("pr_id", req.PullRequestID))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    "NO_CANDIDATE",
					Message: "no active replacement candidate in team",
				},
			})
			return
		}

		// 9. Выполняем переназначение ревьювера
		err = reassigner.ReassignReviewer(req.PullRequestID, req.OldUserID, newReviewerID)
		if err != nil {
			log.Error("failed to reassign reviewer", slog.String("op", op), slog.String("error", err.Error()))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    "INTERNAL_ERROR",
					Message: "failed to reassign reviewer",
				},
			})
			return
		}

		// 10. Обновляем список ревьюеров в объекте PR (заменяем старого на нового)
		updatedReviewers := make([]string, 0, len(pullRequest.AssignedReviewers))
		for _, reviewer := range pullRequest.AssignedReviewers {
			if reviewer != req.OldUserID {
				updatedReviewers = append(updatedReviewers, reviewer)
			}
		}
		updatedReviewers = append(updatedReviewers, newReviewerID)
		pullRequest.AssignedReviewers = updatedReviewers

		log.Info("reviewer reassigned successfully",
			slog.String("op", op),
			slog.String("pr_id", req.PullRequestID),
			slog.String("old_reviewer", req.OldUserID),
			slog.String("new_reviewer", newReviewerID))

		// 11. Возвращаем обновленный PR
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(Response{
			PullRequest: *pullRequest,
			ReplacedBy:  newReviewerID,
		})
	}
}

// contains - вспомогательная функция для проверки наличия элемента в slice
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
