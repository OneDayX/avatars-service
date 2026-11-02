# GophProfile

Сервис загрузки, хранения, обработки и раздачи аватаров. HTTP API построен на Chi, метаданные хранятся в PostgreSQL, файлы — в MinIO/S3, миниатюры и удаление обрабатываются асинхронно через RabbitMQ.

## Запуск

Требуются Docker и Docker Compose:

```bash
docker compose up --build
```

После запуска доступны:

- веб-интерфейс: http://localhost:8080/web/upload;
- healthcheck: http://localhost:8080/health;
- MinIO Console: http://localhost:9001 (`minioadmin` / `minioadmin`);
- RabbitMQ Management: http://localhost:15672 (`guest` / `guest`).

Server и worker собираются из одного multi-stage Dockerfile. PostgreSQL, RabbitMQ и MinIO сохраняют данные в named volumes. Миграции автоматически применяет server при старте.

## API

### Загрузить аватар

```bash
curl -X POST http://localhost:8080/api/v1/avatars \
  -H 'X-User-ID: user@example.com' \
  -F 'file=@avatar.jpg'
```

Поддерживаются JPEG, PNG и WebP, максимум 10 МБ. Формат определяется по magic bytes и проверяется декодированием, а не только по расширению или заявленному MIME-типу. Ответ `201` содержит ID, URL и статус `processing`.

### Получить файл или метаданные

```bash
curl 'http://localhost:8080/api/v1/avatars/AVATAR_UUID?size=300x300' -o avatar.jpg
curl 'http://localhost:8080/api/v1/avatars/AVATAR_UUID/metadata'
curl 'http://localhost:8080/api/v1/users/user@example.com/avatar?size=100x100' -o latest.jpg
curl 'http://localhost:8080/api/v1/users/user@example.com/avatars'
```

Размеры: `100x100`, `300x300`, `original` (по умолчанию). Миниатюры возвращаются как JPEG. Ответы изображений содержат `Cache-Control` и `ETag`, поддерживается `If-None-Match`.

### Удалить

```bash
curl -X DELETE http://localhost:8080/api/v1/avatars/AVATAR_UUID \
  -H 'X-User-ID: user@example.com'

curl -X DELETE http://localhost:8080/api/v1/users/user@example.com/avatar \
  -H 'X-User-ID: user@example.com'
```

Первый запрос удаляет указанный аватар, второй — последний аватар пользователя. В PostgreSQL выполняется мягкое удаление, после чего событие удаления файлов отправляется в RabbitMQ. Удалить чужой аватар нельзя.

## Обработка событий

RabbitMQ использует durable direct exchange `avatars.exchange`, очередь `avatars.processing` и routing keys `avatar.uploaded`, `avatar.deleted`. Сообщения persistent и содержат уникальный Message ID.

Worker создаёт центрированные миниатюры 100×100 и 300×300, затем обновляет статус и ключи в БД. Повторная обработка завершённого аватара не выполняется. Удаление S3-объектов идемпотентно. Ошибки обрабатываются с экспоненциальным backoff (`WORKER_RETRY_DELAY << attempt`); после исчерпания попыток аватар получает статус `failed`.

## Конфигурация

| Переменная | Значение по умолчанию |
|---|---|
| `HTTP_ADDR` | `:8080` |
| `DATABASE_URL` | `postgres://gophprofile:gophprofile@localhost:5432/gophprofile?sslmode=disable` |
| `S3_ENDPOINT` | `localhost:9000` |
| `S3_ACCESS_KEY`, `S3_SECRET_KEY` | `minioadmin` |
| `S3_BUCKET` | `avatars` |
| `S3_USE_SSL` | `false` |
| `RABBITMQ_URL` | `amqp://guest:guest@localhost:5672/` |
| `MAX_FILE_SIZE` | `10485760` |
| `RATE_LIMIT_REQUESTS` | `60` |
| `RATE_LIMIT_WINDOW` | `1m` |
| `CORS_ALLOWED_ORIGINS` | `*` (или список origin через запятую) |
| `WORKER_MAX_RETRIES` | `5` |
| `WORKER_RETRY_DELAY` | `500ms` |

## Проверка качества

```bash
go test -race ./...
make cover
golangci-lint run ./...
```

Для rate limiting используется отдельное фиксированное окно на IP и `X-User-ID`. CORS и максимальный размер запроса включены по умолчанию.
