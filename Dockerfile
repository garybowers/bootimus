FROM gcr.io/distroless/static-debian13

COPY bootimus /bootimus

EXPOSE 69/udp 8080/tcp 8081/tcp

USER root

VOLUME [ "/data" ]

ENTRYPOINT ["/bootimus"]
CMD ["serve"]
