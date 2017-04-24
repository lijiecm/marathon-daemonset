FROM alpine:3.4

ENV RELEASE_VERSION=v0.0.2

ADD https://github.com/nutmegdevelopment/marathon-daemonset/releases/download/${RELEASE_VERSION}/marathon-daemonset-linux-amd64 /marathon-daemonset
RUN chmod +x /marathon-daemonset

CMD ["/marathon-daemonset"]
