package teamSave

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"main.go/internal/models"
)

// Структура запроса
type Request struct {
	TeamName string              `json:"teamname"`
	Members  []models.TeamMember `json:"members"`
}

// Структура ответа
type Response struct {
	Team models.Team `json:"team"`
}

type TeamSaverInterface interface {
	SaveTeamWithUpdate(team models.Team) (bool, error)
	GetTeamMembers(teamName string) ([]models.TeamMember, error)
}

func New(log *slog.Logger, teamSaver TeamSaverInterface) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		const op = "handlers.save.New"

		// 1. Декодируем json из тела запроса
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

		// 2. Валидация - проверяем, что team_name не пустой
		if req.TeamName == "" {
			log.Error("team name is empty", slog.String("op", op))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    "INVALID_REQUEST",
					Message: "team name can't be empty",
				},
			})
			return
		}

		log.Info("saving team", slog.String("op", op), slog.String("team_name", req.TeamName))

		// 3. Создаём объект Team
		team := models.Team{
			TeamName: req.TeamName,
			Members:  req.Members,
		}

		// 4. Пытаемся сохранить/обновить команду
		isNewTeam, err := teamSaver.SaveTeamWithUpdate(team)
		if err != nil {
			log.Error("failed to save team", slog.String("op", op), slog.String("error", err.Error()))
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict) // 409
			json.NewEncoder(w).Encode(models.ErrorResponse{
				Error: models.ErrorDetail{
					Code:    "TEAM_EXISTS",
					Message: "team already exists with same members",
				},
			})
			return
		}

		// 5. Получаем обновлённый список членов команды
		members, err := teamSaver.GetTeamMembers(req.TeamName)
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

		team.Members = members

		// 6. Определяем статус ответа
		statusCode := http.StatusCreated // 201 по умолчанию
		if !isNewTeam {
			statusCode = http.StatusOK // 200 если это было обновление
		}

		log.Info("team saved successfully",
			slog.String("op", op),
			slog.String("team_name", req.TeamName),
			slog.Bool("is_new_team", isNewTeam))

		// 7. Возвращаем ответ
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		json.NewEncoder(w).Encode(Response{
			Team: team,
		})
	}
}
