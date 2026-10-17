# Server image with baked-in pinned scanners and rules.
# Build from the sentinix-ai directory: docker build -f deploy/Dockerfile -t sentinix .
FROM golang:1.27-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS tools
ENV GOWORK=off CGO_ENABLED=0 GOBIN=/out
RUN go install github.com/gitleaks/gitleaks/v8@v8.21.2 && \
    go install github.com/google/osv-scanner/cmd/osv-scanner@v1.9.2

FROM golang:1.27-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS build
WORKDIR /src
ENV GOWORK=off CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -trimpath -o /out/sentinixd ./cmd/sentinixd && \
    go build -trimpath -o /out/sentinix ./cmd/sentinix

FROM python:3.12-alpine@sha256:1b668429b3511ab407d8e00648891631b0b1a4d7e15e3ca70f38ab5b91ad4ab4
RUN apk add --no-cache git ca-certificates && adduser -D -u 10001 sentinix
RUN pip install --no-cache-dir --break-system-packages semgrep==1.108.0
COPY --from=tools /out/ /usr/local/bin/
COPY --from=build /out/ /usr/local/bin/
COPY rules/ /rules/
ENV SENTINIX_SEMGREP_CONFIG=/rules
USER 10001
EXPOSE 8080
ENTRYPOINT ["sentinixd"]
