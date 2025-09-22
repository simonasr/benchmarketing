package controller

import (
	"net/http"
	"time"

	"github.com/simonasr/benchmarketing/redbench/internal/config"
)

// runtimeConfigDTO is the wire shape for the UI to import configuration.
// It intentionally exposes only the fields needed by the UI.
type runtimeConfigDTO struct {
	Version   string         `json:"version"`
	UpdatedAt time.Time      `json:"updatedAt"`
	Config    runtimePayload `json:"config"`
	Targets   []targetDTO    `json:"targets,omitempty"`
}

type runtimePayload struct {
	Redis redisConfigDTO `json:"redis"`
	Test  testConfigDTO  `json:"test"`
}

type redisConfigDTO struct {
	OperationTimeoutMs int   `json:"operationTimeoutMs"`
	Expiration         int32 `json:"expiration"`
	// TLS and connection details are intentionally omitted from import surface
}

type testConfigDTO struct {
	MinClients      int `json:"minClients"`
	MaxClients      int `json:"maxClients"`
	StageIntervalMs int `json:"stageIntervalMs"`
	RequestDelayMs  int `json:"requestDelayMs"`
	KeySize         int `json:"keySize"`
	ValueSize       int `json:"valueSize"`
}

type targetDTO struct {
	RedisURL    string `json:"redisUrl,omitempty"`
	ClusterURL  string `json:"clusterUrl,omitempty"`
	WorkerCount int    `json:"workerCount,omitempty"`
}

const defaultWorkerCount = 1

// RuntimeConfigHandler serves HEAD/GET for UI runtime config import.
// HEAD: 204 if available, 404 if not.
// GET: 200 with DTO payload and ETag for client caching.
func (c *Controller) RuntimeConfigHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodHead:
		if c.config == nil {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	case http.MethodGet:
		if c.config == nil {
			http.NotFound(w, r)
			return
		}
		// Prefer using the running (or latest) job's configuration if available
		cfgSource := c.config
		updatedAt := c.startedAt
		if sel := c.findSelectedJob(); sel != nil {
			if sel.Config != nil {
				cfgSource = sel.Config
			}
			if sel.StartTime != nil {
				updatedAt = *sel.StartTime
			}
		}

		dto := runtimeConfigDTO{
			Version:   "v1",
			UpdatedAt: updatedAt,
			Config: runtimePayload{
				Redis: redisConfigDTO{
					OperationTimeoutMs: cfgSource.Redis.OperationTimeoutMs,
					Expiration:         cfgSource.Redis.Expiration,
				},
				Test: testConfigDTO{
					MinClients:      cfgSource.Test.MinClients,
					MaxClients:      cfgSource.Test.MaxClients,
					StageIntervalMs: cfgSource.Test.StageIntervalMs,
					RequestDelayMs:  cfgSource.Test.RequestDelayMs,
					KeySize:         cfgSource.Test.KeySize,
					ValueSize:       cfgSource.Test.ValueSize,
				},
			},
		}
		// Prefer deriving targets from the current running job (or any existing job) with worker counts
		if ts := c.deriveTargetsFromJobs(); len(ts) > 0 {
			dto.Targets = ts
		} else if baseConn, err := config.LoadRedisConnectionForService(); err == nil && (baseConn.URL != "" || baseConn.ClusterURL != "") {
			// Fallback to base connection from environment
			t := targetDTO{WorkerCount: defaultWorkerCount}
			if baseConn.ClusterURL != "" {
				t.ClusterURL = baseConn.ClusterURL
			} else {
				t.RedisURL = baseConn.URL
			}
			dto.Targets = []targetDTO{t}
		}
		// Disable HTTP caching to ensure preview reflects current job/config
		w.Header().Set("Cache-Control", "no-store")
		writeJSONResponse(w, dto, http.StatusOK)
		return
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

// findSelectedJob returns the currently running job if present, otherwise the most recent job.
func (c *Controller) findSelectedJob() *Job {
	if c == nil || c.jobManager == nil {
		return nil
	}
	jobs := c.jobManager.ListJobs()
	if len(jobs) == 0 {
		return nil
	}
	// Prefer the currently running job if any
	for _, j := range jobs {
		if j.Status == JobStatusRunning {
			return j
		}
	}
	// Otherwise, pick the most recent deterministically by EndTime, then StartTime
	var latest *Job
	var latestTs time.Time
	for _, j := range jobs {
		var ts time.Time
		if j.EndTime != nil {
			ts = *j.EndTime
		} else if j.StartTime != nil {
			ts = *j.StartTime
		}
		if latest == nil || ts.After(latestTs) {
			latest = j
			latestTs = ts
		}
	}
	return latest
}

// deriveTargetsFromJobs attempts to reconstruct targets with worker counts from
// the current running job, or the most recent job if none are running.
func (c *Controller) deriveTargetsFromJobs() []targetDTO {
	if c == nil || c.jobManager == nil {
		return nil
	}
	jobs := c.jobManager.ListJobs()
	if len(jobs) == 0 {
		return nil
	}
	var selected *Job
	for _, j := range jobs {
		if j.Status == JobStatusRunning {
			selected = j
			break
		}
	}
	if selected == nil {
		selected = jobs[len(jobs)-1]
	}
	if selected == nil || len(selected.Assignments) == 0 {
		return nil
	}

	// Preserve stable order of first appearance per target
	order := make([]string, 0, len(selected.Assignments))
	targetsByKey := make(map[string]*targetDTO)

	for _, a := range selected.Assignments {
		if a.RedisConfig == nil {
			continue
		}
		key := ""
		var t targetDTO
		if a.RedisConfig.ClusterURL != "" {
			key = "cluster:" + a.RedisConfig.ClusterURL
			t.ClusterURL = a.RedisConfig.ClusterURL
		} else if a.RedisConfig.URL != "" {
			key = "url:" + a.RedisConfig.URL
			t.RedisURL = a.RedisConfig.URL
		} else {
			continue
		}
		if existing, ok := targetsByKey[key]; ok {
			existing.WorkerCount += 1
		} else {
			t.WorkerCount = 1
			targetsByKey[key] = &t
			order = append(order, key)
		}
	}

	if len(order) == 0 {
		return nil
	}

	out := make([]targetDTO, 0, len(order))
	for _, k := range order {
		if v, ok := targetsByKey[k]; ok {
			out = append(out, *v)
		}
	}
	return out
}
