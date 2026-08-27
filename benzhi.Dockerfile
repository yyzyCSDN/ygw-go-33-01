FROM golang:1.23

ENV GOPROXY=off \
    GOSUMDB=off

WORKDIR /app

COPY go.mod go.sum ./
COPY vendor ./vendor
COPY . .

RUN mkdir -p /app/bin && go build -mod=vendor -o /app/bin/zonedns ./cmd/zonedns

EXPOSE 18080

CMD ["/app/bin/zonedns", "-http", "0.0.0.0:18080"]
