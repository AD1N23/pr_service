package getreview

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"main.go/internal/models"
)

// Response - структура ответа
type Response struct {
	UserID       string                    `json:"user_id"`
	PullRequests []models.PullRequestShort `json:"pull_requests"`
}

// UserReviewInterface - интерфейс для получения PR'ов пользователя
type UserReviewInterface interface {
	CheckUserExists(userID string) error
	GetUserAssignedPullRequests(userID string) ([]models.PullRequestShort, error)
}

// New создаёт handler для GET /users/getReview
func New(log *slog.Logger, userReview UserReviewInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "http-server.handlers.users.getreview.New"

		// 1. Получаем query параметр user_id
		userID := strings.TrimSpace(r.URL.Query().Get("user_id"))

		// 2. Валидация - проверяем, что user_id не пустой
		if userID == "" {
			log.Error("empty user_id in query", slog.String("op", op))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    "INVALID_REQUEST",
					Message: "user_id is required",
				},
			})
			return
		}

		log.Info("getting user assigned PRs",
			slog.String("op", op),
			slog.String("user_id", userID))

		// 3. Проверяем, что пользователь существует
		err := userReview.CheckUserExists(userID)
		if err != nil {
			log.Error("user does not exist", slog.String("op", op), slog.String("error", err.Error()))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    "NOT_FOUND",
					Message: "user not found",
				},
			})
			return
		}

		// 4. Получаем PR'ы, где пользователь назначен ревьювером
		pullRequests, err := userReview.GetUserAssignedPullRequests(userID)
		if err != nil {
			log.Error("failed to get user pull requests", slog.String("op", op), slog.String("error", err.Error()))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    "INTERNAL_ERROR",
					Message: "failed to get pull requests",
				},
			})
			return
		}

		// 5. Если PR'ов нет, возвращаем пустой массив (а не null)
		if pullRequests == nil {
			pullRequests = []models.PullRequestShort{}
		}

		log.Info("user pull requests retrieved successfully",
			slog.String("op", op),
			slog.String("user_id", userID),
			slog.Int("count", len(pullRequests)))

		// 6. Возвращаем успешный ответ
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(Response{
			UserID:       userID,
			PullRequests: pullRequests,
		})
	}
}
