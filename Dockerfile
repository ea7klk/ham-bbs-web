FROM golang:1.23-alpine AS build

WORKDIR /src
RUN apk add --no-cache build-base

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/ham-bbs-web ./cmd/ham-bbs-web

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app

COPY --from=build /out/ham-bbs-web /usr/local/bin/ham-bbs-web
COPY translations.json /usr/local/share/ham-bbs-web/translations.json

ENV BBS_WEB_ADDR=:8080 \
    BBS_DATA_DIR=/var/lib/bbs \
    BBS_DB_FILE=/var/lib/bbs/bbs.sqlite \
    BBS_TRANSLATIONS_FILE=/usr/local/share/ham-bbs-web/translations.json

EXPOSE 8080
VOLUME ["/var/lib/bbs"]

ENTRYPOINT ["/usr/local/bin/ham-bbs-web"]
