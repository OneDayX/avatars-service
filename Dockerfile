FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/worker ./cmd/worker
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/migrate ./cmd/migrate

FROM alpine:3.21
RUN apk --no-cache add ca-certificates tzdata && adduser -D -u 10001 app
WORKDIR /app
COPY --from=builder /out/server /out/worker /out/migrate ./
COPY --from=builder /app/web ./web
COPY --from=builder /app/migrations ./migrations
USER app
EXPOSE 8080
CMD ["./server"]
