#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
mkdir -p examples/spark/jars

mvn -B dependency:copy \
  -Dartifact=org.apache.hadoop:hadoop-aws:3.3.4 \
  -DoutputDirectory="$PWD/examples/spark/jars"
mvn -B dependency:copy \
  -Dartifact=com.amazonaws:aws-java-sdk-bundle:1.12.262 \
  -DoutputDirectory="$PWD/examples/spark/jars"
mvn -B dependency:copy \
  -Dartifact=org.apache.spark:spark-hadoop-cloud_2.12:3.5.6 \
  -DoutputDirectory="$PWD/examples/spark/jars"
