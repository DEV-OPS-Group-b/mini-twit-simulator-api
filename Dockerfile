FROM golang:1.17
WORKDIR /usr/src/api
ENV BACKEND=localhost
COPY minitwit-simulator-api.go ./api.go
COPY go.mod go.sum ./
RUN go mod download
RUN go mod verify
RUN go build
EXPOSE 9000
ENTRYPOINT /usr/src/api -backend=${BACKEND}:8080
