FROM alpine:3.4

ADD config.yml /config.yml
ADD marathon-daemonset /marathon-daemonset
RUN chmod +x /marathon-daemonset

ENTRYPOINT ["/marathon-daemonset"]