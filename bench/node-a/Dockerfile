FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY . .
RUN cd bench/pipe-bridge && go build -o /pipe-bridge .

FROM python:3.12-slim
RUN pip install --no-cache-dir rns==1.1.4
COPY --from=builder /pipe-bridge /bench/pipe-bridge
RUN mkdir -p /root/.reticulum
COPY bench/node-a/config/config /root/.reticulum/config
CMD ["rnsd"]
