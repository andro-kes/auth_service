FROM golang:1.24.2-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /bin/app

FROM alpine:3.21
COPY --from=builder /bin/app /bin/app
EXPOSE 8000
CMD [ "/bin/app" ]