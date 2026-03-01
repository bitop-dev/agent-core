FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY main.go .
RUN go build -ldflags="-s -w" -o tool main.go

FROM alpine:3.21
COPY --from=builder /app/tool /usr/local/bin/tool
ENTRYPOINT ["tool"]
