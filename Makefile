.PHONY: run test cover lint docker-up docker-down helm-lint helm-template k8s-render

run:
	docker compose up --build

test:
	go test -race ./...

cover:
	go test -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

lint:
	golangci-lint run ./...

docker-up:
	docker compose up -d --build

docker-down:
	docker compose down

helm-lint:
	helm lint deploy/helm/gophprofile
	helm lint deploy/helm/gophprofile -f deploy/helm/gophprofile/values-local.yaml
	helm lint deploy/helm/gophprofile -f deploy/helm/gophprofile/values-production.yaml

helm-template:
	helm template gophprofile deploy/helm/gophprofile --namespace gophprofile

k8s-render:
	kubectl kustomize k8s/base
