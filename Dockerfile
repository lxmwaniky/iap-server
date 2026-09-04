FROM golang:1.26.1-bookworm AS build

WORKDIR /src
COPY go.mod go.sum ./
COPY vendor ./vendor
COPY cmd ./cmd
COPY config ./config
COPY internal ./internal

RUN CGO_ENABLED=0 GOOS=linux go build -mod=vendor -trimpath -ldflags="-s -w" -o /out/iap-server ./cmd/api

FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=build /out/iap-server /app/iap-server

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/app/iap-server"]
