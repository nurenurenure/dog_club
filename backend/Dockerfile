# syntax=docker/dockerfile:1
FROM golang:1.26.0-bookworm AS sdk
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .

FROM sdk AS build
RUN mkdir -p /out /storage && chmod 0700 /storage && chown 65532:65532 /storage && \
    CGO_ENABLED=0 go build -trimpath -o /out/api ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath -o /out/migrate ./cmd/migrate && \
    CGO_ENABLED=0 go build -trimpath -o /out/admin ./cmd/admin && \
    CGO_ENABLED=0 go build -trimpath -o /out/maintenance ./cmd/maintenance && \
    CGO_ENABLED=0 go build -trimpath -o /out/healthcheck ./cmd/healthcheck

# The runtime has no Go toolchain and runs as an unprivileged numeric user.
FROM debian:bookworm-slim AS runtime
COPY --from=sdk /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=sdk /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/ /
COPY --from=build --chown=65532:65532 /storage /data/documents
RUN chmod 0700 /data/documents
USER 65532:65532
WORKDIR /data
EXPOSE 8080
ENTRYPOINT ["/api"]
