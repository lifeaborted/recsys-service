FROM golang:1.26-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0

COPY go.mod go.sum ./
RUN ["go", "mod", "download"]
COPY api/ ./api/
COPY internal/ ./internal/

COPY cmd/recommend/ ./cmd/recommend/
RUN ["go", "build", "-mod=readonly", "-trimpath", "-ldflags=-s -w", "-o", "/out/recsys-recommend", "./cmd/recommend"]

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
USER 65532:65532

COPY --from=build /out/recsys-recommend /recsys-recommend
EXPOSE 50051
ENTRYPOINT ["/recsys-recommend"]
