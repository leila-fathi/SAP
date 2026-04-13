# Use official Go image as the base
FROM golang:1.21-alpine AS build

# Set environment variables
ENV GO111MODULE=on
WORKDIR /app

# Copy go.mod and go.sum and download dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the Go application
RUN go build -o game-app main.go

# Use a minimal image for runtime
FROM alpine:latest
WORKDIR /app

# Copy the built binary from the build stage
COPY --from=build /app/game-app .

# Expose the game server port
EXPOSE 8080

# Set default command to run the multiplayer server
CMD ["./game-app", "-mode=mserver", "-players=2", "-timeout=15", "-addr=0.0.0.0:8080"]
