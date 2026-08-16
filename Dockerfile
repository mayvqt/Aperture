FROM golang:1.27rc2 AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/aperture ./cmd/aperture

FROM alpine:3.24

LABEL org.opencontainers.image.source="https://github.com/mayvqt/Aperture"

RUN apk add --no-cache shadow su-exec \
  && addgroup -S aperture \
  && adduser -S aperture -G aperture \
  && mkdir -p /config \
  && chown -R aperture:aperture /config

WORKDIR /app

COPY --from=build /out/aperture /usr/local/bin/aperture
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

RUN chmod +x /usr/local/bin/docker-entrypoint.sh

VOLUME ["/config"]

ENV PUID=99
ENV PGID=100
ENV APERTURE_HTTP_ADDR=:8099
ENV APERTURE_CONFIG_DIR=/config
ENV APERTURE_DB_PATH=/config/aperture.db
ENV APERTURE_LOG_LEVEL=info
ENV APERTURE_TRUSTED_PROXY_CIDRS=127.0.0.1/32

EXPOSE 8099

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["aperture", "serve"]
