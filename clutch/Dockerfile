FROM --platform=linux/amd64 golang:1.25-alpine AS builder
RUN apk add --no-cache ca-certificates
WORKDIR /src
COPY go.mod main.go ./
COPY internal/ internal/
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /clutch .

FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /clutch /clutch
ENTRYPOINT ["/clutch"]
