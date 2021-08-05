FROM golang:1.16

WORKDIR /go/src/app

COPY . .
RUN go build main.go

CMD ["app"]
