# GophProfile

Сервис загрузки, хранения, обработки и раздачи аватаров. HTTP API построен на Chi, метаданные хранятся в PostgreSQL, файлы — в MinIO/S3, миниатюры и удаление обрабатываются асинхронно через RabbitMQ.

- [OpenAPI 3.0](api/openapi.yaml)
- [Архитектура и Kubernetes-компоненты](docs/architecture.md)
- [Helm Chart](deploy/helm/gophprofile)

## Быстрый запуск в Docker Compose

Требуются Docker и Docker Compose:

```bash
docker compose up --build
```

После запуска доступны:

- веб-интерфейс: http://localhost:8080/web/upload;
- healthcheck: http://localhost:8080/health;
- MinIO Console: http://localhost:9001 (`minioadmin` / `minioadmin`);
- RabbitMQ Management: http://localhost:15672 (`guest` / `guest`).

Server и worker собираются из одного multi-stage Dockerfile. PostgreSQL, RabbitMQ и MinIO сохраняют данные в named volumes. Миграции автоматически применяет server при старте (`RUN_MIGRATIONS=true` по умолчанию).

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

## Наблюдаемость

Docker Compose дополнительно запускает Jaeger, Prometheus, Alertmanager, Grafana, Loki и Promtail:

- Grafana: http://localhost:3000 — dashboards **Service Overview** и **Business KPIs and Worker** создаются автоматически;
- Jaeger: http://localhost:16686 — распределённые трейсы HTTP → service → PostgreSQL/S3 → RabbitMQ → worker;
- Prometheus: http://localhost:9090 — метрики, правила и состояние scrape targets;
- Alertmanager: http://localhost:9093 — активные алерты;
- метрики server: http://localhost:8080/metrics;
- метрики worker: http://localhost:8081/metrics.

Promtail читает JSON-логи контейнеров `server` и `worker` и отправляет их в Loki. Логи внутри tracing context содержат `trace_id` и `span_id`; datasource Loki в Grafana связывает их с Jaeger. Собираются HTTP RED-метрики, длительность и количество загрузок, общее использование хранилища, события worker, соединения PostgreSQL и глубина RabbitMQ queue.

Alertmanager получает алерты о 5xx rate выше 10%, p95 latency выше 5 секунд, ошибках загрузок и недоступной очереди. По умолчанию используется локальный receiver без внешних уведомлений; его можно расширить в `docker/alertmanager/alertmanager.yml`.

Для запуска приложения вне Docker tracing по умолчанию выключен. Управление: `OTEL_ENABLED`, `OTEL_EXPORTER_OTLP_ENDPOINT`, `OTEL_SERVICE_NAME`, `APP_ENV`; адрес метрик worker — `METRICS_ADDR`.

## Kubernetes

В `k8s/base` находятся независимые манифесты для Deployment server/worker, Service, Ingress, ConfigMap, Secret, migration Job, HPA, ServiceMonitor, NetworkPolicy, PodDisruptionBudget и ServiceAccount.

Требования кластера:

- NGINX Ingress Controller;
- Metrics Server для HPA;
- CNI с поддержкой NetworkPolicy;
- Prometheus Operator и CRD ServiceMonitor для автоматического сбора метрик;
- доступные PostgreSQL, S3/MinIO и RabbitMQ.

Манифесты содержат только демонстрационные credentials и не предназначены для production. Для ручного запуска:

```bash
kubectl apply -k k8s/base
kubectl -n gophprofile get pods,svc,ingress,hpa
```

Raw Job миграций одноразовый: перед повторным ручным запуском удалите завершённый Job. Для обычных установок и обновлений используйте Helm — он управляет hook Job автоматически.

## Helm

### Локальный кластер Rancher Desktop

Соберите образ средством, который использует выбранный container runtime Rancher Desktop:

```bash
# Для dockerd/moby:
docker build -t avatars-service:latest .

# Для containerd:
nerdctl -n k8s.io build -t avatars-service:latest .
```

Установите зависимости и приложение в один namespace. Пример с Bitnami charts:

```bash
helm upgrade --install postgresql oci://registry-1.docker.io/bitnamicharts/postgresql \
  --namespace gophprofile --create-namespace \
  --set auth.username=gophprofile --set auth.password=gophprofile --set auth.database=gophprofile

helm upgrade --install rabbitmq oci://registry-1.docker.io/bitnamicharts/rabbitmq \
  --namespace gophprofile --set auth.username=guest --set auth.password=guest

helm upgrade --install minio oci://registry-1.docker.io/bitnamicharts/minio \
  --namespace gophprofile --set auth.rootUser=minioadmin \
  --set auth.rootPassword=minioadmin --set defaultBuckets=avatars

helm upgrade --install gophprofile ./deploy/helm/gophprofile \
  --namespace gophprofile \
  -f ./deploy/helm/gophprofile/values-local.yaml
```

Добавьте `127.0.0.1 avatars.local` в `/etc/hosts` и откройте http://avatars.local/web/upload. Если Ingress ещё не настроен:

```bash
kubectl -n gophprofile port-forward svc/gophprofile-server 8080:80
```

`values-local.yaml` отключает ServiceMonitor и NetworkPolicy для простого локального запуска. Чтобы включить их, установите соответствующие операторы и переопределите values.

### Production

Создайте Secret отдельно от релиза и не сохраняйте production credentials в Git:

```bash
kubectl -n gophprofile create secret generic gophprofile-secrets \
  --from-literal=database-url="$DATABASE_URL" \
  --from-literal=s3-access-key="$S3_ACCESS_KEY" \
  --from-literal=s3-secret-key="$S3_SECRET_KEY" \
  --from-literal=rabbitmq-url="$RABBITMQ_URL"
```

Отредактируйте `values-production.yaml`: укажите реальный image registry/tag, DNS, S3 endpoint, OTLP endpoint, namespaces Ingress/Prometheus и ограничения NetworkPolicy. Затем:

```bash
helm upgrade --install gophprofile ./deploy/helm/gophprofile \
  --namespace gophprofile --create-namespace \
  -f ./deploy/helm/gophprofile/values-production.yaml \
  --set metrics.serviceMonitor.additionalLabels.release=monitoring
```

Chart использует `secret.existingSecret=gophprofile-secrets`, поэтому файл Secret в релиз не создаётся. ConfigMap и Secret подключаются через переменные окружения; секретные значения не выводятся в `helm template` при использовании внешнего Secret.

### Миграции, обновление и удаление

`./migrate` запускается Helm hooks `post-install` и `post-upgrade`. Бинарник читает только `DATABASE_URL` и `MIGRATIONS_DIR`, поэтому ошибки в настройках S3, rate limit или worker не влияют на hook. Ошибки содержат этап создания пула или применения миграций. Миграции идемпотентны и защищены PostgreSQL advisory lock. Успешный hook Job удаляется автоматически; при ошибке релиз завершается неуспешно, а Job остаётся для диагностики:

```bash
kubectl -n gophprofile get jobs
kubectl -n gophprofile logs job/gophprofile-migration
helm rollback gophprofile REVISION --namespace gophprofile
```

Для rolling update используется expand/contract. Сначала добавляются обратно совместимые таблицы, nullable-колонки или безопасные default: старая и новая версии приложения могут работать с такой схемой одновременно. Удаление, переименование и другие несовместимые изменения выполняются только отдельной contract-миграцией после полного вывода старой версии из эксплуатации.

Удаление приложения не удаляет внешние PostgreSQL, S3 и RabbitMQ:

```bash
helm uninstall gophprofile --namespace gophprofile
```

## Масштабирование и безопасность

HPA изменяет число server pod от 2 до 10 (в production values — от 3 до 20) при CPU выше 70% или памяти выше 80%. Для worker задаётся независимый `worker.replicaCount`.

Все контейнеры работают от UID 10001 с read-only root filesystem, отключённым privilege escalation, `drop: ALL` и seccomp. ServiceAccount не монтирует токен и не имеет Role/ClusterRole, поэтому приложение не может обращаться к Kubernetes API. NetworkPolicy запрещает лишний Ingress/Egress; список портов и namespaces настраивается через values.

`/live` используется для liveness, `/ready` — для readiness и проверяет PostgreSQL, S3 и RabbitMQ. При SIGTERM server прекращает принимать соединения и завершает активные запросы, worker заканчивает обработку и возвращает незавершённые сообщения в очередь. `preStop`, `terminationGracePeriodSeconds` и `SHUTDOWN_TIMEOUT` согласованы для graceful shutdown.

## Конфигурация

| Переменная | Значение по умолчанию |
|---|---|
| `HTTP_ADDR` | `:8080` |
| `METRICS_ADDR` | `:8081` |
| `DATABASE_URL` | `postgres://gophprofile:gophprofile@localhost:5432/gophprofile?sslmode=disable` |
| `RUN_MIGRATIONS` | `true` |
| `S3_ENDPOINT` | `localhost:9000` |
| `S3_ACCESS_KEY`, `S3_SECRET_KEY` | `minioadmin` |
| `S3_BUCKET` | `avatars` |
| `S3_USE_SSL` | `false` |
| `RABBITMQ_URL` | `amqp://guest:guest@localhost:5672/` |
| `MAX_FILE_SIZE` | `10485760` |
| `RATE_LIMIT_REQUESTS` | `60` |
| `RATE_LIMIT_WINDOW` | `1m` |
| `CORS_ALLOWED_ORIGINS` | `*` (или список origin через запятую) |
| `SHUTDOWN_TIMEOUT` | `10s` |
| `WORKER_MAX_RETRIES` | `5` |
| `WORKER_RETRY_DELAY` | `500ms` |

## Проверка качества

```bash
go test -race ./...
make cover
golangci-lint run ./...
docker build -t avatars-service:latest .
kubectl kustomize k8s/base
helm lint deploy/helm/gophprofile
helm template gophprofile deploy/helm/gophprofile
```

Для rate limiting используется отдельное фиксированное окно на IP и `X-User-ID`. CORS и максимальный размер запроса включены по умолчанию.
