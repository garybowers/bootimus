# Debian 13 (Trixie) distroless for minimal attack surface
FROM gcr.io/distroless/static-debian13:nonroot

WORKDIR /app

# Copy pre-built binary
COPY bootimus /app/bootimus

# Expose ports
# Note: port 69 requires root/CAP_NET_BIND_SERVICE
EXPOSE 69/udp 8080/tcp 8081/tcp

# Switch to root to allow binding to port 69
# In production, use CAP_NET_BIND_SERVICE capability instead
USER root

ENTRYPOINT ["/app/bootimus"]
CMD ["serve"]
