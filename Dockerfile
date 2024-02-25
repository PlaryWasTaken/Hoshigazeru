FROM golang:1.22
LABEL author="Plary"

WORKDIR /usr/src/app

# pre-copy/cache go.mod for pre-downloading dependencies and only redownloading them in subsequent builds if they change
COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN go build -buildvcs=false -v -o /usr/local/bin ./...
EXPOSE 8080
WORKDIR /usr/local/bin
CMD ["./Hoshigazeru"]