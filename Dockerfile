# syntax=docker/dockerfile:1.7

##########
# Build
##########
FROM golang:1.25-alpine AS build
WORKDIR /src

# Optional tools for private modules; harmless otherwise
RUN apk add --no-cache git

# 1) Cache modules separately for faster rebuilds
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# 2) Copy the rest and build a static binary
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags="-s -w" -o /out/loxone-philips-hue .

##########
# Runtime (distroless, non-root)
##########
FROM gcr.io/distroless/static:nonroot
WORKDIR /app
COPY --from=build /out/loxone-philips-hue /app/loxone-philips-hue

USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/app/loxone-philips-hue"]
