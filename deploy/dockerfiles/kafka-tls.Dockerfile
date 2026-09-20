# Kafka with TLS + client authentication + ArrowheadPrincipalBuilder plugin.
#
# At connection time, after TLS handshake, the broker calls ArrowheadPrincipalBuilder
# which queries PIP to check cert validity. Revoked certs are rejected with an
# AuthenticationException before any Kafka protocol interaction (D2' enforcement).
#
# Build context: repo root (Arrowhead-520-evol/)

# -- Stage 1: Build the Java plugin -------------------------------------------
FROM maven:3.9.6-eclipse-temurin-11 AS plugin-builder

WORKDIR /build
COPY services/kafka-principal-builder/pom.xml .
RUN mvn dependency:go-offline -q

COPY services/kafka-principal-builder/src/ ./src/
RUN mvn package -q -DskipTests

# -- Stage 2: Kafka broker with plugin ----------------------------------------
FROM confluentinc/cp-kafka:7.6.1

USER root
RUN microdnf install -y openssl

# Install the plugin JAR
COPY --from=plugin-builder /build/target/kafka-principal-builder.jar /opt/kafka-plugins/kafka-principal-builder.jar

# Install the entrypoint script
COPY deploy/dockerfiles/kafka-tls-entrypoint.sh /kafka-tls-entrypoint.sh
RUN chmod +x /kafka-tls-entrypoint.sh

USER 1000
ENTRYPOINT ["/kafka-tls-entrypoint.sh"]
