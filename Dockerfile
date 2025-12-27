FROM gcr.io/distroless/static-debian13

WORKDIR /app

COPY bootimus /app/bootimus

EXPOSE 69/udp 8080/tcp 8081/tcp

USER root

ENTRYPOINT ["/app/bootimus"]
CMD ["serve"]
