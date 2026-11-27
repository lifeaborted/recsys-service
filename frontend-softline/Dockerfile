FROM golang:1.26-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0

COPY go.mod go.sum ./
RUN ["go", "mod", "download"]
COPY api/ ./api/
COPY internal/ ./internal/

COPY cmd/api/ ./cmd/api/
RUN ["go", "build", "-mod=readonly", "-trimpath", "-ldflags=-s -w", "-o", "/out/recsys-api", "./cmd/api"]

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
USER 65532:65532

WORKDIR /app
COPY --from=build /out/recsys-api /recsys-api
COPY api/openapi.yaml ./api/openapi.yaml
COPY api/examples/recommendations.full.json api/examples/recommendations.partial.json ./api/examples/
ENV RECSYS_ADDR=0.0.0.0:8080 RECSYS_MOCK=1
EXPOSE 8080
ENTRYPOINT ["/recsys-api"]
