# Build stage
FROM golang:1.21-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -v -o loxone-philips-hue .

# Final image
FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/loxone-philips-hue .
EXPOSE 8080
ENTRYPOINT ["./loxone-philips-hue"]