package merge

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"main.go/internal/models"
)

// Request - структура запроса
type Request struct {
	PullRequestID string `json:"pull_request_id"`
}

// Response - структура ответа
type Response struct {
	PullRequest models.PullRequest `json:"pr"`
}

// PRMergerInterface - интерфейс для merge операции
type PRMergerInterface interface {
	MergePullRequest(pullRequestID string) (*models.PullRequest, error)
}

// New создаёт handler для POST /pullRequest/merge
func New(log *slog.Logger, prMerger PRMergerInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "http-server.handlers.pr.merge.New"

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

		// 2. Валидация - проверяем, что pull_request_id не пустой
		if req.PullRequestID == "" {
			log.Error("pull_request_id is empty", slog.String("op", op))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    "INVALID_REQUEST",
					Message: "pull request contains empty fields",
				},
			})
			return
		}

		log.Info("merging pull request", slog.String("op", op), slog.String("pr_id", req.PullRequestID))

		// 3. Мержим PR
		pullRequest, err := prMerger.MergePullRequest(req.PullRequestID)
		if err != nil {
			log.Error("failed to merge PR", slog.String("op", op), slog.String("pr_id", req.PullRequestID), slog.String("error", err.Error()))
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

		log.Info("pull request merged successfully", slog.String("op", op), slog.String("pr_id", req.PullRequestID))

		// 4. Возвращаем merged PR
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) // 200
		json.NewEncoder(w).Encode(Response{
			PullRequest: *pullRequest,
		})
	}
}
