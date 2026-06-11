package nodeagent

const (
	DefaultTailLines    = 300
	DefaultSinceSeconds = 1800
	DefaultLimitBytes   = 1024 * 1024
	MaxTailLines        = 1000
	MaxSinceSeconds     = 86400
	MaxLimitBytes       = 5 * 1024 * 1024
	DefaultDiskTopLimit = 20
	MaxDiskTopLimit     = 100
	DefaultDiskMaxDepth = 2
	MaxDiskMaxDepth     = 4
)

type LogRequest struct {
	Host         string
	Container    string
	TailLines    int
	SinceSeconds int
	LimitBytes   int
	Timestamps   bool
}

type ContainerLog struct {
	Host         string `json:"host,omitempty"`
	Container    string `json:"container"`
	TailLines    int    `json:"tail_lines"`
	SinceSeconds int    `json:"since_seconds"`
	LimitBytes   int    `json:"limit_bytes"`
	Truncated    bool   `json:"truncated"`
	Text         string `json:"text"`
}

type HostDiskRequest struct {
	Host  string
	Limit int
	Depth int
}

type HostDiskResponse struct {
	Host          string                  `json:"host,omitempty"`
	Filesystems   []HostFilesystem        `json:"filesystems"`
	TopPaths      []HostPathUsage         `json:"top_paths"`
	Docker        HostDockerDisk          `json:"docker"`
	ContainerLogs []HostContainerLogUsage `json:"container_logs"`
	CleanupPlan   []HostCleanupPlanItem   `json:"cleanup_plan"`
	Limits        HostDiskLimits          `json:"limits"`
	Warnings      []string                `json:"warnings,omitempty"`
}

type HostFilesystem struct {
	Path        string  `json:"path"`
	Mountpoint  string  `json:"mountpoint,omitempty"`
	Device      string  `json:"device,omitempty"`
	FSType      string  `json:"fstype,omitempty"`
	TotalBytes  uint64  `json:"total_bytes"`
	FreeBytes   uint64  `json:"free_bytes"`
	AvailBytes  uint64  `json:"avail_bytes"`
	UsedPercent float64 `json:"used_percent"`
}

type HostPathUsage struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	Depth     int    `json:"depth"`
	Error     string `json:"error,omitempty"`
}

type HostDockerDisk struct {
	Available                  bool  `json:"available"`
	LayersSizeBytes            int64 `json:"layers_size_bytes"`
	ImagesSizeBytes            int64 `json:"images_size_bytes"`
	ImagesReclaimableBytes     int64 `json:"images_reclaimable_bytes"`
	ContainersSizeRwBytes      int64 `json:"containers_size_rw_bytes"`
	ContainersSizeRootFsBytes  int64 `json:"containers_size_rootfs_bytes"`
	VolumesSizeBytes           int64 `json:"volumes_size_bytes"`
	VolumesReclaimableBytes    int64 `json:"volumes_reclaimable_bytes"`
	BuildCacheSizeBytes        int64 `json:"build_cache_size_bytes"`
	BuildCacheReclaimableBytes int64 `json:"build_cache_reclaimable_bytes"`
	ApproxReclaimableBytes     int64 `json:"approx_reclaimable_bytes"`
}

type HostContainerLogUsage struct {
	Container  string            `json:"container"`
	ID         string            `json:"id,omitempty"`
	LogPath    string            `json:"log_path,omitempty"`
	MappedPath string            `json:"mapped_path,omitempty"`
	SizeBytes  int64             `json:"size_bytes"`
	LogDriver  string            `json:"log_driver,omitempty"`
	LogOptions map[string]string `json:"log_options,omitempty"`
	Warning    string            `json:"warning,omitempty"`
}

type HostCleanupPlanItem struct {
	ID                string `json:"id"`
	Risk              string `json:"risk"`
	Summary           string `json:"summary"`
	Evidence          string `json:"evidence,omitempty"`
	Recommendation    string `json:"recommendation"`
	MinValidation     string `json:"min_validation"`
	ExecutionBoundary string `json:"execution_boundary"`
}

type HostDiskLimits struct {
	AllowedPaths []string `json:"allowed_paths"`
	MaxDepth     int      `json:"max_depth"`
	TopLimit     int      `json:"top_limit"`
	ReadOnly     bool     `json:"read_only"`
}

func BoundedLogRequest(req LogRequest) LogRequest {
	req.TailLines = clamp(defaultInt(req.TailLines, DefaultTailLines), 1, MaxTailLines)
	req.SinceSeconds = clamp(defaultInt(req.SinceSeconds, DefaultSinceSeconds), 1, MaxSinceSeconds)
	req.LimitBytes = clamp(defaultInt(req.LimitBytes, DefaultLimitBytes), 1, MaxLimitBytes)
	return req
}

func BoundedHostDiskRequest(req HostDiskRequest) HostDiskRequest {
	req.Limit = clamp(defaultInt(req.Limit, DefaultDiskTopLimit), 1, MaxDiskTopLimit)
	req.Depth = clamp(defaultInt(req.Depth, DefaultDiskMaxDepth), 0, MaxDiskMaxDepth)
	return req
}

func defaultInt(value, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}

func clamp(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}
