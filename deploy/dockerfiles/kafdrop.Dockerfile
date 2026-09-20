# Kafdrop Dockerfile.
# Extends the official Kafdrop image with a startup script that generates
# kafka.properties using PEM certs from the shared certs volume (mTLS Kafka).
# Build context: repo root (Arrowhead-520-evol/)

FROM obsidiandynamics/kafdrop:latest

COPY deploy/kafdrop/entrypoint.sh /kafdrop-entrypoint.sh
RUN chmod +x /kafdrop-entrypoint.sh

ENTRYPOINT ["/kafdrop-entrypoint.sh"]
