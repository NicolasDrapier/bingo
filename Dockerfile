# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.27.2-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY web/ ./web/
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
    go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/bingo ./cmd/server

FROM --platform=$BUILDPLATFORM alpine:3.23 AS prepare

RUN addgroup -S -g 10001 bingo && adduser -S -u 10001 -G bingo bingo \
    && mkdir /data && chown bingo:bingo /data

FROM alpine:3.23

COPY --from=prepare /etc/passwd /etc/passwd
COPY --from=prepare /etc/group /etc/group
COPY --from=prepare --chown=10001:10001 /data /data
COPY --from=build /out/bingo /usr/local/bin/bingo

USER bingo
WORKDIR /data
ENV PORT=8080 DB_PATH=/data/bingo.db
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/usr/local/bin/bingo"]
