FROM golang:latest as build
WORKDIR /app
COPY src/ src
WORKDIR /app/src
RUN go mod download
RUN go build -o ../os-serverlist-sync

FROM golang:latest
WORKDIR /app
COPY --from=build /app/os-serverlist-sync os-serverlist-sync
COPY run.sh .
ENTRYPOINT ["/bin/bash", "/app/run.sh"]