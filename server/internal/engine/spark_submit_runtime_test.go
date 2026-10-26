package engine

import (
	"context"
	"fmt"
	"linha/server/internal/domain"
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"
	"time"
)

// Exercise the Spark image entrypoint and Ivy initialization, before user main().
func TestSparkSubmitContainerUsers(t *testing.T) {
	image := os.Getenv("LINHA_TEST_SPARK_IMAGE")
	if image == "" {
		t.Skip("set LINHA_TEST_SPARK_IMAGE to a Spark 3.5.6 worker image to test container startup")
	}
	settings, err := ParseSpark(domain.EngineSpec{Version: "3.5.6", Settings: domain.JSON(map[string]any{})})
	if err != nil {
		t.Fatal(err)
	}
	conf := settings.runtimeConf()
	keys := make([]string, 0, len(conf))
	for key := range conf {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for name, user := range map[string]string{"image-user": "185:185", "unmapped-user": "42424:10001"} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
			defer cancel()
			container := fmt.Sprintf("linha-spark-runtime-%d", time.Now().UnixNano())
			t.Cleanup(func() { _, _ = exec.Command("docker", "rm", "-f", container).CombinedOutput() })
			args := []string{"run", "--rm", "--name", container, "--user", user, "--cap-drop=ALL", "--security-opt=no-new-privileges", "--network", "none", "--hostname", "localhost",
				"-e", "SPARK_LOCAL_IP=127.0.0.1", "-e", "SPARK_LOCAL_HOSTNAME=localhost", "-e", "SPARK_DRIVER_BIND_ADDRESS=127.0.0.1",
				image, "driver", "--master", "local[1]", "--conf", "spark.driver.host=127.0.0.1", "--conf", "spark.driver.memory=512m"}
			for _, key := range keys {
				args = append(args, "--conf", key+"="+conf[key])
			}
			args = append(args, "--class", "org.apache.spark.examples.SparkPi", "local:///opt/spark/examples/jars/spark-examples_2.12-3.5.6.jar", "1")
			output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
			if err != nil {
				t.Fatalf("Spark driver failed for %s: %v\n%s", user, err, output)
			}
			if !strings.Contains(string(output), "Pi is roughly") {
				t.Fatalf("Spark job did not complete for %s:\n%s", user, output)
			}
		})
	}
}
