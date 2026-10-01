package engine

import (
	"bytes"
	"encoding/json"
	"fmt"
	"linha/server/internal/domain"
	"regexp"
	"strconv"
	"strings"
)

type DriverSettings struct {
	Min             int    `json:"minDrivers"`
	Max             int    `json:"maxDrivers"`
	Concurrency     int    `json:"maxConcurrentRequestsPerDriver"`
	QueueThreshold  int    `json:"queueThreshold"`
	WaitSeconds     int    `json:"waitSeconds"`
	CooldownSeconds int    `json:"cooldownSeconds"`
	IdleSeconds     int    `json:"idleSeconds"`
	MemoryMi        int    `json:"memoryMi,omitempty"`
	Memory          string `json:"memory,omitempty"`
	MemoryOverhead  string `json:"memoryOverhead,omitempty"`
	CoreRequest     string `json:"coreRequest,omitempty"`
	CoreLimit       string `json:"coreLimit,omitempty"`
	JavaOptions     string `json:"javaOptions,omitempty"`
	Cores           int    `json:"cores"`
}
type ExecutorSettings struct {
	ExecutorIdleTimeout       string `json:"executorIdleTimeout,omitempty"`
	CachedExecutorIdleTimeout string `json:"cachedExecutorIdleTimeout,omitempty"`
	ShuffleTrackingTimeout    string `json:"shuffleTrackingTimeout,omitempty"`
	Dynamic                   bool   `json:"dynamicAllocation"`
	Min                       int    `json:"minExecutors"`
	Initial                   int    `json:"initialExecutors"`
	Max                       int    `json:"maxExecutors"`
	Instances                 int    `json:"instances"`
	MemoryMi                  int    `json:"memoryMi,omitempty"`
	Memory                    string `json:"memory,omitempty"`
	MemoryOverhead            string `json:"memoryOverhead,omitempty"`
	CoreRequest               string `json:"coreRequest,omitempty"`
	CoreLimit                 string `json:"coreLimit,omitempty"`
	JavaOptions               string `json:"javaOptions,omitempty"`
	Cores                     int    `json:"cores"`
}
type SparkApplication struct {
	MainClass           string   `json:"mainClass"`
	MainApplicationFile string   `json:"mainApplicationFile"`
	Arguments           []string `json:"arguments,omitempty"`
}
type SparkUISettings struct {
	Enabled bool `json:"enabled"`
	Port    *int `json:"port,omitempty"`
}
type SparkSettings struct {
	UI          *SparkUISettings  `json:"ui,omitempty"`
	Application *SparkApplication `json:"application,omitempty"`
	Drivers     DriverSettings    `json:"drivers"`
	Executors   ExecutorSettings  `json:"executors"`
	Kubernetes  *PodTemplates     `json:"kubernetes,omitempty"`
}

func ParseSpark(spec domain.EngineSpec) (SparkSettings, error) {
	c := SparkSettings{Drivers: DriverSettings{Min: 1, Max: 1, Concurrency: 1, QueueThreshold: 1, WaitSeconds: 5, CooldownSeconds: 10, IdleSeconds: 60, Cores: 1}, Executors: ExecutorSettings{Min: 0, Initial: 1, Max: 4, Instances: 1, Cores: 1}}
	if spec.Version != "3.5.6" {
		return c, domain.Bad("supported Spark version is 3.5.6")
	}
	d := json.NewDecoder(bytes.NewReader(spec.Settings))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, domain.Bad("invalid Spark settings: " + err.Error())
	}
	if err := normalizeResources(&c.Drivers.MemoryMi, &c.Drivers.Memory, &c.Drivers.MemoryOverhead, c.Drivers.Cores, &c.Drivers.CoreRequest, &c.Drivers.CoreLimit, c.Drivers.JavaOptions, c.Application != nil); err != nil {
		return c, err
	}
	if err := normalizeResources(&c.Executors.MemoryMi, &c.Executors.Memory, &c.Executors.MemoryOverhead, c.Executors.Cores, &c.Executors.CoreRequest, &c.Executors.CoreLimit, c.Executors.JavaOptions, c.Application != nil); err != nil {
		return c, err
	}
	if a := c.Application; a != nil {
		if !regexp.MustCompile(`^[a-zA-Z_$][a-zA-Z0-9_$.]*$`).MatchString(a.MainClass) || !strings.HasPrefix(a.MainApplicationFile, "local:///") || !strings.HasSuffix(a.MainApplicationFile, ".jar") || len(a.Arguments) > 128 {
			return c, domain.Bad("application requires a JVM mainClass and local:/// path to its image JAR")
		}
	}
	if err := c.normalizeRuntimeSettings(); err != nil {
		return c, err
	}
	v := c.Drivers
	e := c.Executors
	if v.Min < 0 || v.Max < v.Min || v.Max < 1 || v.Max > 16 || v.Concurrency < 1 || v.Concurrency > 32 || v.QueueThreshold < 1 || v.WaitSeconds < 1 || v.CooldownSeconds < 1 || v.IdleSeconds < 1 || v.Cores < 1 || v.Cores > 32 {
		return c, domain.Bad("invalid Spark driver bounds")
	}
	if e.Min < 0 || e.Initial < e.Min || e.Max < e.Initial || e.Max < 1 || e.Max > 128 || e.Instances < 1 || e.Instances > 128 || e.Cores < 1 || e.Cores > 32 {
		return c, domain.Bad("invalid Spark executor bounds")
	}
	return c, nil
}
func (c SparkSettings) Conf(namespace, image, host, pod, serviceAccount string) map[string]string {
	values := map[string]string{"spark.master": "k8s://https://kubernetes.default.svc:443", "spark.submit.deployMode": "client", "spark.app.name": "linha-" + pod, "spark.kubernetes.namespace": namespace, "spark.kubernetes.container.image": image, "spark.kubernetes.driver.pod.name": pod, "spark.driver.host": host, "spark.driver.bindAddress": "0.0.0.0", "spark.driver.port": "7078", "spark.blockManager.port": "7079", "spark.kubernetes.authenticate.driver.serviceAccountName": serviceAccount, "spark.executor.memory": fmt.Sprintf("%dm", c.Executors.MemoryMi), "spark.executor.cores": fmt.Sprint(c.Executors.Cores), "spark.executor.instances": fmt.Sprint(c.Executors.Instances)}
	for key, value := range c.runtimeConf() {
		values[key] = value
	}
	// Task parallelism alone does not impose a Kubernetes CPU limit.
	values["spark.kubernetes.executor.request.cores"] = fmt.Sprint(c.Executors.Cores)
	values["spark.kubernetes.executor.limit.cores"] = fmt.Sprint(c.Executors.Cores)
	if c.Executors.Dynamic {
		delete(values, "spark.executor.instances")
		values["spark.dynamicAllocation.minExecutors"] = fmt.Sprint(c.Executors.Min)
		values["spark.dynamicAllocation.initialExecutors"] = fmt.Sprint(c.Executors.Initial)
		values["spark.dynamicAllocation.maxExecutors"] = fmt.Sprint(c.Executors.Max)
	}
	return values
}

func memoryMi(value string) (int, error) {
	m := regexp.MustCompile(`^([0-9]+)([mMgG])$`).FindStringSubmatch(value)
	if m == nil {
		return 0, domain.Bad("memory must use Spark units m or g, for example 1024m or 10g")
	}
	n, err := strconv.Atoi(m[1])
	if err != nil || n > 131072 {
		return 0, domain.Bad("memory exceeds supported bounds")
	}
	if strings.EqualFold(m[2], "g") {
		n *= 1024
	}
	return n, nil
}
func normalizeResources(legacy *int, memory, overhead *string, cores int, request, limit *string, java string, application bool) error {
	if *legacy != 0 && *memory != "" {
		return domain.Bad("use memory or deprecated memoryMi, not both")
	}
	n := *legacy
	if *memory != "" {
		var err error
		n, err = memoryMi(*memory)
		if err != nil {
			return err
		}
	}
	if n == 0 {
		if *memory != "" {
			return domain.Bad("heap memory must be positive")
		}
		n = 1024
	}
	if n < 512 || n > 131072 {
		return domain.Bad("heap memory must be between 512m and 128g")
	}
	if application || *memory != "" {
		*memory = fmt.Sprintf("%dm", n)
		*legacy = 0
	} else {
		*legacy = n
	}
	if *overhead != "" {
		o, err := memoryMi(*overhead)
		if err != nil || o < 1 || o > 131072 {
			return domain.Bad("invalid memoryOverhead")
		}
		*overhead = fmt.Sprintf("%dm", o)
	} else if application {
		*overhead = fmt.Sprintf("%dm", max(384, n/10))
	}
	if *request == "" {
		*request = fmt.Sprint(cores)
	}
	if *limit == "" {
		*limit = fmt.Sprint(cores)
	}
	cpu := func(v string) (float64, error) {
		if strings.HasSuffix(v, "m") {
			n, e := strconv.ParseFloat(strings.TrimSuffix(v, "m"), 64)
			return n / 1000, e
		}
		return strconv.ParseFloat(v, 64)
	}
	r, e := cpu(*request)
	if e != nil || !(r > 0 && r <= 32) {
		return domain.Bad("invalid coreRequest")
	}
	l, e := cpu(*limit)
	if e != nil || !(l >= r && l <= 32) {
		return domain.Bad("coreLimit must be at least coreRequest and at most 32")
	}
	*request = strconv.FormatFloat(r, 'f', -1, 64)
	*limit = strconv.FormatFloat(l, 'f', -1, 64)
	if strings.Contains(java, "-Xmx") || strings.Contains(java, "-Xms") || strings.Contains(java, "MaxRAM") {
		return domain.Bad("configure JVM heap through memory, not javaOptions")
	}
	return nil
}
