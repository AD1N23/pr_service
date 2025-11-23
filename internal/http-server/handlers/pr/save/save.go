package PrSave

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"main.go/internal/models"
)

type Request struct {
	PullRequestID   string `json:"pull_request_id"`
	PullRequestName string `json:"pull_request_name"`
	AuthorID        string `json:"author_id"`
}

type Response struct {
	PullRequest models.PullRequest `json:"pr"`
}

type PRSeverInterface interface {
	CreatePullRequest(models.PullRequest) error
	GetActiveTeamMembers(string) ([]string, error)
	CheckAuthorExist(string) error
}

func New(log *slog.Logger, prSaver PRSeverInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "http-server.handlers.pr.save.New"
		// 1.Декодируем json
		var req Request
		var assignedMembers []string
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			log.Error("failed to decode request", slog.String("op", op), slog.String("error", err.Error()))
			// Если не удалось декодировать отправляем ошибку и выходим из обработчика
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

		//2. Валидация
		if req.PullRequestID == "" || req.AuthorID == "" || req.PullRequestName == "" {
			log.Error("pull_request exists empty fields", slog.String("op", op))
			// Если не удалось декодировать отправляем ошибку и выходим из обработчика
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest) // 400
			json.NewEncoder(w).Encode(models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    "INVALID_REQUEST",
					Message: "pull_request exists empty fields",
				},
			})
			return
		}
		pullRequest := models.PullRequest{
			PullRequestID:   req.PullRequestID,
			PullRequestName: req.PullRequestName,
			AuthorID:        req.AuthorID,
		}
		// 3. Получаем активных членов команды
		assignedMembers, err = prSaver.GetActiveTeamMembers(req.AuthorID)
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

		if len(assignedMembers) == 0 {
			log.Error("no active team members found", slog.String("op", op))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    "NO_CANDIDATE",
					Message: "no active team members to assign as reviewers",
				},
			})
			return
		}

		// 4. Берём максимум 2 ревьювера (обрезаем slice)
		maxReviewers := 2
		if len(assignedMembers) < 2 {
			maxReviewers = len(assignedMembers)
		}
		assignedMembers = assignedMembers[:maxReviewers] // ✅ просто обрезаем

		// 5. Создаём PR
		pullRequest = models.PullRequest{
			PullRequestID:     req.PullRequestID,
			PullRequestName:   req.PullRequestName,
			AuthorID:          req.AuthorID,
			Status:            "OPEN",
			AssignedReviewers: assignedMembers,
		}

		err = prSaver.CreatePullRequest(pullRequest)
		if err != nil {
			log.Error("failed to create PR", slog.String("op", op), slog.String("error", err.Error()))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			json.NewEncoder(w).Encode(models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    "PR_EXISTS",
					Message: "pull_request already exist",
				},
			})
			return
		}

		log.Info("PR created successfully", slog.String("op", op), slog.String("pr_id", req.PullRequestID))

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(Response{
			PullRequest: pullRequest,
		})

	}
}
