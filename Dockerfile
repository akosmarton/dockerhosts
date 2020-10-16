FROM golang:1.14 as builder

WORKDIR /app/dockerhosts

COPY . .

RUN go mod download
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build

FROM alpine

COPY --from=builder /app/dockerhosts /app/

CMD ["/app/dockerhosts"]

ENV DOCKER_HOST=unix:///data/docker.sock HOSTS_FILE=/data/hosts
