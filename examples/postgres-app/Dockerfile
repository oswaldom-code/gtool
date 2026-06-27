# Build
FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/postgres-app .

# Run
FROM alpine:3.20
RUN adduser -D -u 10001 app
USER app
COPY --from=build /bin/postgres-app /bin/postgres-app
EXPOSE 8080
ENTRYPOINT ["/bin/postgres-app"]
