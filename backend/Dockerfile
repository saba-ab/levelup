# One image, five entrypoints (api, worker, dispatcher, scheduler, migrate).
# Static binaries on distroless: no shell, no package manager, non-root.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ ./cmd/...

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ /app/
USER nonroot:nonroot
ENTRYPOINT ["/app/api"]
