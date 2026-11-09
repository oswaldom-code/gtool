# Build
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /bin/mountebank-app .

# Run
FROM alpine:3.20
RUN adduser -D -u 10001 app
USER app
COPY --from=build /bin/mountebank-app /bin/mountebank-app
EXPOSE 8080
ENTRYPOINT ["/bin/mountebank-app"]
