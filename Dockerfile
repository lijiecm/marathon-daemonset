FROM alpine:3.4

ENV MARATHON_DAEMONSET_VERSION=0.0.1

ADD https://github.com/nutmegdevelopment/marathon-daemonset/releases/download/v${MARATHON_DAEMONSET_VERSION}/marathon-daemonset-linux-amd64 /marathon-daemonset
RUN chmod +x /marathon-daemonset

CMD ["/marathon-daemonset"]