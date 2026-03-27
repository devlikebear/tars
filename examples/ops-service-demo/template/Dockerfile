FROM golang:1.25.6-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN go build -o /out/ops-service-demo ./cmd/ops-service-demo

FROM alpine:3.22

RUN adduser -D -u 10001 appuser
COPY --from=build /out/ops-service-demo /usr/local/bin/ops-service-demo
USER appuser
EXPOSE 18080

ENV PORT=18080
ENV SYNTHETIC_CHECK_INTERVAL=5s

CMD ["ops-service-demo"]
