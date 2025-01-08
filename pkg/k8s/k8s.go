package k8s

type RayJob struct {
	APIVersion string            `yaml:"apiVersion"`
	Kind       string            `yaml:"kind"`
	Metadata   Metadata          `yaml:"metadata"`
	Spec       RayJobSpec        `yaml:"spec"`
}

type Metadata struct {
	Name      string            `yaml:"name"`
	Namespace string            `yaml:"namespace"`
	Labels    map[string]string `yaml:"labels"`
}

type RayJobSpec struct {
	ShutdownAfterJobFinishes bool            `yaml:"shutdownAfterJobFinishes"`
	Entrypoint               string          `yaml:"entrypoint"`
	RayClusterSpec           RayClusterSpec  `yaml:"rayClusterSpec"`
}

type RayService struct {
	APIVersion string          `yaml:"apiVersion"`
	Kind       string          `yaml:"kind"`
	Metadata   Metadata        `yaml:"metadata"`
	Spec       RayServiceSpec  `yaml:"spec"`
}

type Job struct {
	APIVersion string      `yaml:"apiVersion"`
	Kind       string      `yaml:"kind"`
	Metadata   Metadata    `yaml:"metadata"`
	Spec       JobSpec     `yaml:"spec"`
}

type JobSpec struct {
	Template    PodTemplate `yaml:"template"`
	BackoffLimit int        `yaml:"backoffLimit"`
}


type RayServiceSpec struct {
	ServeConfigV2   string           `yaml:"serveConfigV2"`
	RayClusterConfig RayClusterConfig `yaml:"rayClusterConfig"`
}

type RayClusterSpec struct {
	RayVersion              string            `yaml:"rayVersion"`
	EnableInTreeAutoscaling bool              `yaml:"enableInTreeAutoscaling"`
	HeadGroupSpec           HeadGroupSpec     `yaml:"headGroupSpec"`
	WorkerGroupSpecs        []WorkerGroupSpec `yaml:"workerGroupSpecs"`
}

type RayClusterConfig struct {
	RayVersion              string            `yaml:"rayVersion"`
	EnableInTreeAutoscaling bool           `yaml:"enableInTreeAutoscaling"`
	HeadGroupSpec           HeadGroupSpec  `yaml:"headGroupSpec"`
	WorkerGroupSpecs        []WorkerGroupSpec `yaml:"workerGroupSpecs"`
}

type HeadGroupSpec struct {
	RayStartParams map[string]string `yaml:"rayStartParams"`
	Template       PodTemplate       `yaml:"template"`
}

type WorkerGroupSpec struct {
	Replicas     int               `yaml:"replicas"`
	MinReplicas  int               `yaml:"minReplicas"`
	MaxReplicas  int               `yaml:"maxReplicas"`
	GroupName    string            `yaml:"groupName"`
	RayStartParams map[string]string `yaml:"rayStartParams"`
	Template     PodTemplate       `yaml:"template"`
}

type PodTemplate struct {
	Spec PodSpec `yaml:"spec"`
}

type PodSpec struct {
	Containers []Container `yaml:"containers"`
	Volumes    []Volume    `yaml:"volumes"`
}

type Container struct {
	Name            string              `yaml:"name"`
	Image           string              `yaml:"image"`
	ImagePullPolicy string              `yaml:"imagePullPolicy"`
	Env             []EnvVar            `yaml:"env"`
	Ports           []ContainerPort     `yaml:"ports,omitempty"`
	Resources       ResourceRequirements `yaml:"resources"`
	VolumeMounts    []VolumeMount       `yaml:"volumeMounts"`
}

type EnvVar struct {
	Name      string         `yaml:"name"`
	ValueFrom *ValueFrom     `yaml:"valueFrom,omitempty"`
	Value     string         `yaml:"value,omitempty"`
}

type ValueFrom struct {
	SecretKeyRef *SecretKeySelector `yaml:"secretKeyRef,omitempty"`
}

type SecretKeySelector struct {
	Name string `yaml:"name"`
	Key  string `yaml:"key"`
}

type ContainerPort struct {
	ContainerPort int    `yaml:"containerPort"`
	Name          string `yaml:"name"`
}

type ResourceRequirements struct {
	Limits   map[string]string `yaml:"limits"`
	Requests map[string]string `yaml:"requests"`
}

type VolumeMount struct {
	MountPath string `yaml:"mountPath"`
	Name      string `yaml:"name"`
	SubPath   string `yaml:"subPath,omitempty"`
}

type Volume struct {
	Name      string             `yaml:"name"`
	ConfigMap *ConfigMapVolume   `yaml:"configMap,omitempty"`
	Secret    *SecretVolume      `yaml:"secret,omitempty"`
	EmptyDir  *EmptyDirVolume    `yaml:"emptyDir,omitempty"`
}

type ConfigMapVolume struct {
	Name  string             `yaml:"name"`
	Items []KeyToPathMapping `yaml:"items"`
}

type SecretVolume struct {
	SecretName string             `yaml:"secretName"`
	Items      []KeyToPathMapping `yaml:"items"`
}

type EmptyDirVolume struct {
	Medium    string `yaml:"medium,omitempty"`
	SizeLimit string `yaml:"sizeLimit,omitempty"`
}

type KeyToPathMapping struct {
	Key  string `yaml:"key"`
	Path string `yaml:"path"`
}

type ClusterQueue struct {
	APIVersion string              `yaml:"apiVersion"`
	Kind       string              `yaml:"kind"`
	Metadata   Metadata            `yaml:"metadata"`
	Spec       ClusterQueueSpec    `yaml:"spec"`
}

type ClusterQueueSpec struct {
	NamespaceSelector map[string]interface{} `yaml:"namespaceSelector"`
	ResourceGroups    []ResourceGroup        `yaml:"resourceGroups"`
}

type ResourceGroup struct {
	CoveredResources []string           `yaml:"coveredResources"`
	Flavors          []ResourceFlavorSpec `yaml:"flavors"`
}

type ResourceFlavorSpec struct {
	Name      string           `yaml:"name"`
	Resources []ResourceQuota  `yaml:"resources"`
}

type ResourceQuota struct {
	Name         string `yaml:"name"`
	NominalQuota string `yaml:"nominalQuota"`
}

type LocalQueue struct {
	APIVersion string          `yaml:"apiVersion"`
	Kind       string          `yaml:"kind"`
	Metadata   Metadata        `yaml:"metadata"`
	Spec       LocalQueueSpec  `yaml:"spec"`
}

type LocalQueueSpec struct {
	ClusterQueue string `yaml:"clusterQueue"`
}

type ResourceFlavor struct {
	APIVersion string              `yaml:"apiVersion"`
	Kind       string              `yaml:"kind"`
	Metadata   Metadata            `yaml:"metadata"`
	Spec       ResourceFlavorSpec  `yaml:"spec"`
}


