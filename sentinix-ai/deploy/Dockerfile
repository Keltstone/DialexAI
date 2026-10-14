# Server image. Build from the sentinix-ai directory: docker build -f deploy/Dockerfile -t sentinix .
FROM golang:1.27 AS build
WORKDIR /src
ENV GOWORK=off CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -o /out/sentinixd ./cmd/sentinixd && go build -trimpath -o /out/sentinix ./cmd/sentinix

FROM alpine:3.20
RUN apk add --no-cache git ca-certificates && adduser -D -u 10001 sentinix
COPY --from=build /out/ /usr/local/bin/
USER 10001
EXPOSE 8080
ENTRYPOINT ["sentinixd"]
