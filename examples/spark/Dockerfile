FROM apache/spark:3.5.6-scala2.12-java17-python3-ubuntu
USER root
RUN mkdir -p /opt/linha/bin && chown -R 185:0 /opt/linha
COPY --chown=185:0 examples/jvm/target/linha-examples_2.12-0.1.0-SNAPSHOT.jar /opt/linha/linha-examples.jar
COPY --chmod=755 examples/spark/worker /opt/linha/bin/worker
COPY --chown=185:0 examples/spark/jars/*.jar /opt/spark/jars/
USER 185
