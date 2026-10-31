# Jenkins controller used as a CI agent for apps tested with gtool. It runs
# builds on the built-in node and drives the host Docker daemon through the
# mounted socket.
FROM jenkins/jenkins:lts-jdk21

USER root
COPY --from=docker:27-cli /usr/local/bin/docker /usr/local/bin/docker
# gtool is installed like any other CLI the pipelines need.
COPY gtool /usr/local/bin/gtool
USER jenkins

COPY plugins.txt /usr/share/jenkins/ref/plugins.txt
RUN jenkins-plugin-cli --plugin-file /usr/share/jenkins/ref/plugins.txt

COPY casc.yml /usr/share/jenkins/ref/casc.yml
ENV CASC_JENKINS_CONFIG=/usr/share/jenkins/ref/casc.yml
