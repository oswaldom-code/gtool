# Build
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/mysql-app .

# Run
FROM alpine:3.20
RUN adduser -D -u 10001 app
USER app
WORKDIR /app
COPY --from=build /bin/mysql-app /bin/mysql-app
COPY data ./data
EXPOSE 8080
ENTRYPOINT ["/bin/mysql-app"]
