FROM golang:1.23.5-bookworm

WORKDIR /app
COPY ./go.mod /app/go.mod
COPY ./go.sum /app/go.sum
COPY ./main.go /app/main.go
COPY ./notify.go /app/notify.go
COPY ./calendar.go /app/calendar.go
COPY ./repository.go /app/repository.go
RUN go mod download
RUN go build -o /app/main .
ENTRYPOINT [ "/app/main" ]
