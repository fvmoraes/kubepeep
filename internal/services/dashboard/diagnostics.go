package dashboard

type LatencyPercentilesDTO struct {
	P50Milliseconds int64 `json:"p50Milliseconds"`
	P95Milliseconds int64 `json:"p95Milliseconds"`
	P99Milliseconds int64 `json:"p99Milliseconds"`
	MaxMilliseconds int64 `json:"maxMilliseconds"`
	Samples         int   `json:"samples"`
}

type ResourceSyncDiagnosticDTO struct {
	Resource string                `json:"resource"`
	Latency  LatencyPercentilesDTO `json:"latency"`
}

type PerformanceDiagnosticsDTO struct {
	APIServerLatency  LatencyPercentilesDTO       `json:"apiServerLatency"`
	CacheHitRatio     *float64                    `json:"cacheHitRatio"`
	ActiveWatches     int64                       `json:"activeWatches"`
	RequestsPerMinute float64                     `json:"requestsPerMinute"`
	Responses429      uint64                      `json:"responses429"`
	WatchReconnects   uint64                      `json:"watchReconnects"`
	ResourceSync      []ResourceSyncDiagnosticDTO `json:"resourceSync"`
}

type CacheCoverageDiagnosticDTO struct {
	Topic            string   `json:"topic"`
	State            string   `json:"state"`
	Complete         bool     `json:"complete"`
	LoadedNamespaces []string `json:"loadedNamespaces"`
}

type NamespaceDiagnosticsDTO struct {
	Namespace               string                  `json:"namespace"`
	Resources               map[string]int          `json:"resources"`
	Problems                map[ProblemSeverity]int `json:"problems"`
	Restarts                *int64                  `json:"restarts"`
	ListLatencyMilliseconds *int64                  `json:"listLatencyMilliseconds"`
	Complete                bool                    `json:"complete"`
}

type KubePeepDiagnosticsDTO struct {
	ResourceCacheEntries   int   `json:"resourceCacheEntries"`
	ResourceCacheBytes     int   `json:"resourceCacheBytes"`
	CollectionCacheEntries int   `json:"collectionCacheEntries"`
	CollectionCacheBytes   int   `json:"collectionCacheBytes"`
	CursorBytes            int64 `json:"cursorBytes"`
	ActiveWatches          int64 `json:"activeWatches"`
}

type ClusterDiagnosticsDTO struct {
	KubernetesVersion *string                `json:"kubernetesVersion"`
	Namespaces        int                    `json:"namespaces"`
	Totals            map[string]int         `json:"totals"`
	KubePeep          KubePeepDiagnosticsDTO `json:"kubepeep"`
}

type DiagnosticsDTO struct {
	Generation    string                       `json:"generation"`
	CollectedAt   string                       `json:"collectedAt"`
	Performance   PerformanceDiagnosticsDTO    `json:"performance"`
	Cluster       ClusterDiagnosticsDTO        `json:"cluster"`
	Namespaces    []NamespaceDiagnosticsDTO    `json:"namespaces"`
	CacheCoverage []CacheCoverageDiagnosticDTO `json:"cacheCoverage"`
	Complete      bool                         `json:"complete"`
	Errors        []PartialError               `json:"errors"`
}
