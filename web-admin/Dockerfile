FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /headscale-admin .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=builder /headscale-admin /usr/local/bin/headscale-admin
EXPOSE 9080
ENTRYPOINT ["headscale-admin"]
