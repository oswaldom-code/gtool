# Build
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/mongodb-app .

# Run
FROM alpine:3.20
RUN adduser -D -u 10001 app
USER app
WORKDIR /app
COPY --from=build /bin/mongodb-app /bin/mongodb-app
COPY data ./data
EXPOSE 8080
ENTRYPOINT ["/bin/mongodb-app"]
