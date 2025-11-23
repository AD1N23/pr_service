# Используем официальный образ Go для сборки
FROM golang:1.25.4-alpine AS builder

# Загружаем зависимости
WORKDIR /app
COPY go.mod go.sum ./
COPY config/local.yaml /app/config/local.yaml
RUN go mod download

# Копируем исходники и собираем бинарь
COPY . .
RUN go build -o pr-reviewer ./cmd/main.go

# Запускающий образ — минимальный, только запускем скомпилированный бинарь
FROM alpine:latest

# Устанаваливаем необходимые библиотеки (например, если нужны SSL или DNS)
RUN apk --no-cache add ca-certificates

# Копируем бинарь из builder слоя
WORKDIR /app
COPY --from=builder /app/pr-reviewer .
COPY --from=builder /app/config/local.yaml /app/config/local.yaml

# Пробрасываем порт
EXPOSE 8080

# Запускаем приложение
CMD ["./pr-reviewer"]
