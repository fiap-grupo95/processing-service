FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /processing-service ./main.go

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /processing-service /processing-service
ENTRYPOINT ["/processing-service"]
