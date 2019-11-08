FROM golang:alpine as builder
ADD . /go/src/github.com/silenceper/deny-empty-nodeselector-webhook/
RUN export GO111MODULE=on && export GOPROXY=https://goproxy.io \
  && cd /go/src/github.com/silenceper/deny-empty-nodeselector-webhook/ \
  && go get -v \
  && CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o app .

FROM alpine
MAINTAINER silenceper <silenceper@gmail.com>
COPY --from=builder /go/src/github.com/silenceper/deny-empty-nodeselector-webhook/app /bin/app
ENTRYPOINT ["/bin/app","-port=8080"]
EXPOSE 8080