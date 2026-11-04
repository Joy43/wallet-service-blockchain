# Build with the REPOSITORY ROOT as context (auth depends on the shared ./gen module):
#   docker build -f services/auth/Dockerfile -t yourorg/auth-service:latest .

# ---- build stage ----
FROM golang:1.26-alpine AS build
WORKDIR /src

# Shared generated gRPC code (referenced by `replace ../../gen` in services/auth/go.mod)
COPY gen/ gen/

# Cache dependencies separately from source code
COPY services/auth/go.mod services/auth/go.sum services/auth/
WORKDIR /src/services/auth
RUN go mod download

COPY services/auth/ /src/services/auth/
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/auth ./cmd/auth

# ---- runtime stage: tiny, no shell, non-root ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/auth /auth
EXPOSE 8081 9090
USER nonroot:nonroot
ENTRYPOINT ["/auth"]
