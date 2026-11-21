# AceMate: build the UI, then one static Go binary with the UI embedded, then a minimal
# runtime image (no shell, runs as a non-root user).
#
#   docker build --build-arg VERSION=27.0.1 -t acemate .
#   docker compose up            # with Postgres, see docker-compose.yml

FROM node:26-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26-alpine AS build
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /web/dist ./web/dist
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags="-s -w -X github.com/andriantam/ace-mate/internal/version.Version=${VERSION}" \
      -o /out/server ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /server
ENV APP_ENV=production PORT=8080
EXPOSE 8080
USER nonroot
# The image has no curl, so the binary checks itself: GET /readyz on its own port.
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD ["/server", "healthcheck"]
ENTRYPOINT ["/server"]
