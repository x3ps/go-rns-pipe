FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY . .
RUN cd examples/tcp && go build -o /rns-tcp-iface .

FROM python:3.12-slim
RUN pip install --no-cache-dir rns==1.1.4
COPY --from=builder /rns-tcp-iface /bench/rns-tcp-iface
RUN mkdir -p /root/.reticulum
COPY bench/node-b/config/config /root/.reticulum/config
COPY bench/node-b/reflector.py /bench/reflector.py
CMD ["python3", "/bench/reflector.py"]
