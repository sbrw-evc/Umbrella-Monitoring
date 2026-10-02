# One image with the Go binary and the built web UI.
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.24-alpine AS backend
WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /umbrella ./cmd/umbrella

FROM gcr.io/distroless/static-debian12
COPY --from=backend /umbrella /umbrella
COPY --from=web /web/dist /web
ENV UMBRELLA_WEB_DIR=/web UMBRELLA_ADDR=:8080
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/umbrella"]
