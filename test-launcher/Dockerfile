FROM maven:3.9.9-eclipse-temurin-21

ENV HOME=/tmp \
    MAVEN_CONFIG=/opt/m2 \
    MAVEN_ARGS="-Dmaven.repo.local=/opt/m2/repository"

WORKDIR /app
COPY . .
RUN chmod +x scripts/run.bash \
 && mvn -B test \
 && rm -rf /app/target \
 && mkdir -p /app/target/karate-reports \
 && chmod -R a+rwX /app /opt/m2

VOLUME /app/features

CMD ["bash", "-c", "./scripts/run.bash"]
