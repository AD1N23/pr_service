package teamGet

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"main.go/internal/models"
)

// Response - структура ответа (возвращаем команду)
type Response struct {
	Team []models.TeamMember `json:"team"` // встраиваем Team напрямую, чтобы JSON был плоским
}

// ErrorResponse - структура ответа с ошибкой
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// TeamGetterInterface - интерфейс для получения команды из БД
type TeamGetterInterface interface {
	GetTeamMembers(teamName string) ([]models.TeamMember, error)
}

// New создаёт handler для GET /team/get
func New(log *slog.Logger, teamGetter TeamGetterInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "http-server.handlers.teamGet.New"

		// 1. Получаем query-параметр teamname из URL
		teamName := r.URL.Query().Get("team_name")

		log.Info("getting team", slog.String("op", op), slog.String("teamname", teamName))

		// 2. Валидация - параметр обязателен
		if teamName == "" {
			log.Error("teamname parameter is missing", slog.String("op", op))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(ErrorResponse{
				Error: ErrorDetail{
					Code:    "INVALID_REQUEST",
					Message: "teamname parameter is required",
				},
			})
			return
		}

		// 3. Получаем команду из БД
		team, err := teamGetter.GetTeamMembers(teamName)
		if err != nil {
			log.Error("team not found", slog.String("op", op), slog.String("teamname", teamName))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound) // 404
			json.NewEncoder(w).Encode(ErrorResponse{
				Error: ErrorDetail{
					Code:    "NOTFOUND",
					Message: "team not found",
				},
			})
			return
		}

		log.Info("team found", slog.String("op", op), slog.String("teamname", teamName))

		// 4. Возвращаем команду
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)     // 200
		json.NewEncoder(w).Encode(&team) // возвращаем team напрямую
	}
}
