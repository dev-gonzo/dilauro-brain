FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /brain ./cmd/server

FROM alpine:3.21
RUN addgroup -S brain && adduser -S brain -G brain
COPY --from=build /brain /usr/local/bin/brain
USER brain
EXPOSE 47180
ENTRYPOINT ["brain"]
