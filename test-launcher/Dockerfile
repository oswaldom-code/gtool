# Test launcher image (Karate + Java helpers) for gtool component testing.
#
# Build trick: `mvn test` runs with ZERO features because TestLauncher only runs
# `classpath:launcher/features`, a symlink to /app/features which is empty at
# build time (the real features are bind-mounted by gtool at runtime). This
# pre-caches all Maven dependencies into the image and compiles the helpers
# without executing any feature.
#
# The container MUST run as root: gtool's Karate runner writes reports as root
# and then chowns them back via a short-lived container using this same image.
FROM maven:3.9.9-eclipse-temurin-21

WORKDIR /app
COPY . .
RUN chmod +x scripts/run.bash && mvn -B test

VOLUME /app/features

CMD ["bash", "-c", "./scripts/run.bash"]
