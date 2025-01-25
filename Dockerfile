FROM golang:1.23.5-bookworm

WORKDIR /app
COPY ./go.mod /app/go.mod
COPY ./go.sum /app/go.sum
COPY ./main.go /app/main.go
RUN go mod download
RUN go build -o /app/main /app/main.go
ENTRYPOINT [ "/app/main" ]
