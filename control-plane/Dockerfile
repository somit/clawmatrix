FROM --platform=linux/amd64 golang:1.22-alpine AS builder
RUN apk add --no-cache ca-certificates git
WORKDIR /src
COPY . .
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /control-plane .

FROM gcr.io/distroless/static-debian12
COPY --from=builder /control-plane /control-plane
ENTRYPOINT ["/control-plane"]
