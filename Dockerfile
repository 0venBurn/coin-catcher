FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY scraper ./scraper
RUN CGO_ENABLED=0 go build -o /coin-catcher-scraper ./cmd/scraper

FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=build /coin-catcher-scraper /usr/local/bin/coin-catcher-scraper
ENTRYPOINT ["coin-catcher-scraper"]
