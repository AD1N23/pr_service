package setactive

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"main.go/internal/models"
)

type Request struct {
	UserID   string `json:"user_id"`
	IsActive bool   `json:"is_active"`
}

type Response struct {
	User models.User `json:"user"`
}

type UserUpdaterInterface interface {
	SetUserActive(userID string, isActive bool) (*models.User, error)
}

func New(log *slog.Logger, userUpdater UserUpdaterInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "http-server.handlers.users.set_active.New"

		// 1. Декодируем json из тела запроса в req
		var req Request
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			log.Error("failed to decode request", slog.String("op", op))
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
		log.Info("setting user active status",
			slog.String("op", op),
			slog.String("user_id", req.UserID),
			slog.Bool("is_active", req.IsActive))

		// 2. Валидация
		if req.UserID == "" {
			log.Error("user_id is empty", slog.String("op", op))
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

		// 3. Обновляем статус
		user, err := userUpdater.SetUserActive(req.UserID, req.IsActive)
		if err != nil {
			log.Error("user not found", slog.String("op", op), slog.String("user_id", req.UserID))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound) // 404
			json.NewEncoder(w).Encode(models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    "NOTFOUND",
					Message: "user not found",
				},
			})
			return
		}
		log.Info("user status updated", slog.String("op", op), slog.String("user_id", req.UserID))
		if user == nil {
			log.Error("user is nil after update", slog.String("op", op))
			// обработка ошибки
			return
		}

		// 4. Возвращаем обновлённого пользователя
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK) // 200
		json.NewEncoder(w).Encode(Response{
			User: *user,
		})
	}
}
