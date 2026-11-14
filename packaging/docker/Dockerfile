FROM golang:1.27-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/islandd ./cmd/islandd && \
    CGO_ENABLED=0 go build -o /out/island ./cmd/island

FROM alpine:3.20
RUN adduser -D -h /var/lib/islandweave islandweave
COPY --from=builder /out/islandd /usr/local/bin/islandd
COPY --from=builder /out/island /usr/local/bin/island
USER islandweave
VOLUME ["/var/lib/islandweave"]
EXPOSE 47472
ENTRYPOINT ["/usr/local/bin/islandd", "--data-dir", "/var/lib/islandweave", "--listen", "0.0.0.0:47472"]
