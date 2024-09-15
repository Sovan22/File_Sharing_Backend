
FROM golang:1.23 as build

ENV CGO_ENABLED=1

RUN apt-get update && apt-get install -y sqlite3 libsqlite3-dev

WORKDIR /app

COPY go.mod ./
COPY go.sum ./

RUN go mod download

COPY . .

RUN go build -o file-sharing-backend main.go

FROM golang:1.23

WORKDIR /app

COPY --from=build /app/file-sharing-backend /app/
COPY --from=build /lib/x86_64-linux-gnu/libsqlite3.so.0 /lib/x86_64-linux-gnu/

EXPOSE 8080

CMD ["/app/file-sharing-backend"]

