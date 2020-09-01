FROM golang:1.15
ARG EMAIL=devops@syncano.com
ENV GOPROXY=https://proxy.golang.org
WORKDIR /opt/build

COPY go.mod go.sum ./
RUN go mod download
