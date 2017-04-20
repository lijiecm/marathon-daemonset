FROM alpine:3.4

ADD marathon-daemonset /marathon-daemonset
RUN chmod +x /marathon-daemonset

ENTRYPOINT ["/marathon-daemonset"]