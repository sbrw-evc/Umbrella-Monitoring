.PHONY: build test run dev-api dev-web image

build:
	cd web && npm ci && npm run build
	cd backend && go build -o umbrella ./cmd/umbrella

test:
	cd backend && go vet ./... && go test ./...
	cd web && npm run typecheck

run: build
	cd backend && ./umbrella -web ../web/dist

dev-api:
	cd backend && go run ./cmd/umbrella

dev-web:
	cd web && npm run dev

image:
	docker build -t umbrella:mvp .
