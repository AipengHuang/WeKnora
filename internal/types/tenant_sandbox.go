package types

import (
	"database/sql/driver"
	"encoding/json"
	"log"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/utils"
)

// TenantSandboxConfig is one named sandbox backend a workspace maintains.
//
// It is self-contained: provider fields are not inherited from process
// environment. Leaving a required provider field empty is rejected on save.
type TenantSandboxConfig struct {
	// SandboxType is cube, e2b, or docker; disabled is the hidden policy row.
	SandboxType string `json:"sandbox_type,omitempty"`

	// ── 通用配置（跨后端生效）──────────────────────────────────

	// DefaultTimeoutSec is the per-execution timeout in seconds. 0 uses the
	// program's built-in default.
	DefaultTimeoutSec int `json:"default_timeout_sec,omitempty"`

	// AllowPrivateEndpoints permits this workspace config to reach RFC1918 or
	// loopback cluster endpoints. Link-local/cloud-metadata addresses remain
	// blocked. It is explicit in the UI instead of hidden in process env.
	AllowPrivateEndpoints bool `json:"allow_private_endpoints,omitempty"`

	// EnvVars are additional environment variables injected into every
	// sandbox created for this tenant. 🔒 Values are encrypted at rest.
	// These become visible to all scripts running in the tenant's
	// sandboxes — do not place secrets here that scripts must not access.
	EnvVars map[string]string `json:"env_vars,omitempty"`

	// VolumeMount configures an optional shared volume mounted into every
	// sandbox created for this tenant. Currently used for tenant-installed
	// skills, but the configuration itself is skill-agnostic and can serve
	// any volume-mount use case (shared datasets, pre-installed toolchains,
	// etc.).
	VolumeMount *VolumeMountConfig `json:"volume_mount,omitempty"`

	// SkillImage points at the snapshot that carries this config's installed
	// skills. Empty means "use the base template". Written only by the skill
	// install/remove path: MergeSandboxConfigForUpdate ignores client values
	// so a settings-form save cannot wipe or plant the pointer.
	SkillImage *SkillImageConfig `json:"skill_image,omitempty"`

	// SkillRollout decides whether sessions that already hold a sandbox of
	// this config rebuild after a skill install or removal. Empty and
	// SkillRolloutNextTurn rebuild on the next chat turn. SkillRolloutNewSession
	// leaves those sandboxes on the previous image; only sessions that start
	// afterwards boot the new snapshot.
	SkillRollout string `json:"skill_rollout,omitempty"`

	// ── 后端专属配置（同一时刻只有一个生效，由 SandboxType 决定）───

	Cube   *CubeSandboxConfig   `json:"cube,omitempty"`
	E2B    *E2BSandboxConfig    `json:"e2b,omitempty"`
	Docker *DockerSandboxConfig `json:"docker,omitempty"`
}

// CubeSandboxConfig addresses one CubeSandbox deployment. APIURL, ProxyURL,
// SandboxDomain and TemplateID are all required; APIKey is optional because the
// common single-node setup runs unauthenticated.
type CubeSandboxConfig struct {
	APIURL        string `json:"api_url,omitempty"`
	ProxyURL      string `json:"proxy_url,omitempty"`
	SandboxDomain string `json:"sandbox_domain,omitempty"`
	APIKey        string `json:"api_key,omitempty"` // 加密
	TemplateID    string `json:"template_id,omitempty"`

	// HTTPTimeoutSec bounds each HTTP call to the sandbox control plane.
	// 0 means use the built-in default (30s), never the deployment's value.
	HTTPTimeoutSec int `json:"http_timeout_sec,omitempty"`

	CubeSandboxTTLSeconds int `json:"cube_sandbox_ttl_seconds,omitempty"`

	// DNSServers are Cube template nameserver IPs. Empty uses Cubelet's default.
	DNSServers []string `json:"dns_servers,omitempty"`
}

// E2BSandboxConfig addresses one E2B-protocol control plane: E2B Cloud, a
// self-hosted E2B Infrastructure, or any E2B-compatible implementation
// (CubeSandbox, Agent-Sandbox, …). APIKey and TemplateID are required; APIURL
// and SandboxDomain are optional because go-e2b resolves both on its own when
// they are empty.
type E2BSandboxConfig struct {
	APIURL        string `json:"api_url,omitempty"`
	SandboxDomain string `json:"sandbox_domain,omitempty"`
	APIKey        string `json:"api_key,omitempty"` // 加密
	TemplateID    string `json:"template_id,omitempty"`

	// ProxyURL is the data-plane gateway that fronts envd. E2B Cloud resolves
	// "<port>-<sandboxID>.<sandbox_domain>" through public DNS and TLS, so it
	// needs no value here. Self-hosted E2B-compatible control planes usually
	// serve every sandbox from one gateway address and expect the sandbox
	// authority in the Host header; setting this makes WeKnora dial the
	// gateway directly instead of requiring wildcard DNS and a certificate
	// for the sandbox domain. An "http://" gateway also downgrades the
	// data-plane scheme, which the E2B SDK otherwise pins to https.
	ProxyURL string `json:"proxy_url,omitempty"`

	// HTTPTimeoutSec bounds each HTTP call to the sandbox control plane.
	// 0 means use the built-in default (30s), never the deployment's value.
	HTTPTimeoutSec int `json:"http_timeout_sec,omitempty"`

	E2BSandboxTTLSeconds int `json:"e2b_sandbox_ttl_seconds,omitempty"`
}

// DockerSandboxConfig addresses one Docker daemon. Image is required and plays
// the role a template ID plays for the MicroVM backends: every session
// container is created from it.
//
// The daemon endpoint is deliberately the only connection field, and TLS
// material is referenced by path rather than stored here. Client certificates
// are deployment infrastructure mounted onto the WeKnora host; keeping them
// out of the database keeps them out of backups, exports and API responses.
type DockerSandboxConfig struct {
	Image string `json:"image,omitempty"`

	// Host is the daemon endpoint in DOCKER_HOST form. Empty means the local
	// unix socket.
	Host string `json:"host,omitempty"`

	// TLSCertPath is a directory on the WeKnora host containing ca.pem,
	// cert.pem and key.pem. Required when Host is a TCP endpoint.
	TLSCertPath string `json:"tls_cert_path,omitempty"`

	// CPULimit is the number of CPU cores one sandbox may use. 0 uses the
	// built-in default.
	CPULimit float64 `json:"cpu_limit,omitempty"`

	// MemoryLimitMB caps one sandbox's memory. 0 uses the built-in default.
	MemoryLimitMB int `json:"memory_limit_mb,omitempty"`

	// PidsLimit caps how many processes one sandbox may run. 0 uses the
	// built-in default.
	PidsLimit int `json:"pids_limit,omitempty"`

	// NetworkMode is the Docker network sandboxes join: "bridge" (default) or
	// "none" for no egress. Nothing else is accepted — host and container:
	// modes share another namespace outright, and a named network is usually
	// the deployment's own compose network, which would put the sandbox next
	// to Postgres and Redis.
	NetworkMode string `json:"network_mode,omitempty"`

	// Runtime selects an alternative OCI runtime such as "runsc" (gVisor).
	// Empty uses the daemon default.
	Runtime string `json:"runtime,omitempty"`

	// IdleTTLSeconds is how long a session container may go unused before it
	// is reclaimed. The daemon has no idle timeout of its own, so this is what
	// stops an abandoned session from pinning host memory indefinitely.
	IdleTTLSeconds int `json:"idle_ttl_seconds,omitempty"`

	// HTTPTimeoutSec bounds each Engine API call. 0 uses the built-in default.
	HTTPTimeoutSec int `json:"http_timeout_sec,omitempty"`
}

// VolumeMountConfig configures a shared volume mount into every sandbox
// created for this tenant. Currently implemented for E2B volumes (used to
// share tenant-installed skills across sandbox sessions), but the schema is
// intentionally skill-agnostic so it can serve other volume-based use cases
// in the future.
type VolumeMountConfig struct {
	// Enabled toggles the volume mount for this tenant.
	Enabled bool `json:"enabled"`

	// MountPath is the sandbox-internal path where the volume is mounted.
	// Default: /weknora/tenant/skills (customizable per use case).
	MountPath string `json:"mount_path,omitempty"`

	// Provider identifies the volume backend. Currently "e2b" or "cube".
	Provider string `json:"provider,omitempty"`

	// VolumeID is the provider-specific volume identifier, populated after
	// EnsureVolume / CreateVolume succeeds.
	VolumeID string `json:"volume_id,omitempty"`

	// VolumeName is the human-readable volume name, e.g.
	// "weknora-tenant-<id>-skills".
	VolumeName string `json:"volume_name,omitempty"`

	// VolumeOwnerFingerprint = sha256(provider + APIKey + APIURL).
	// Used to detect when the tenant switched to a different backend or
	// API key, at which point the volume is no longer reachable and must
	// be recreated.
	VolumeOwnerFingerprint string `json:"volume_owner_fingerprint,omitempty"`
}

const (
	// SkillRolloutNextTurn rebuilds a live session's sandbox on its next chat
	// turn after the skill image changes. This is the default.
	SkillRolloutNextTurn = "next_turn"
	// SkillRolloutNewSession leaves live sandboxes on the previous image.
	// Only a session that starts after the install or removal boots the new one.
	SkillRolloutNewSession = "new_session"
)

// RebuildsExistingOnSkillChange reports whether an install or removal should
// mark already-bound sandboxes stale. Unknown values fail toward rebuilding
// so a corrupted row cannot pin every session on a retired image.
func (c *TenantSandboxConfig) RebuildsExistingOnSkillChange() bool {
	if c == nil {
		return true
	}
	return strings.TrimSpace(c.SkillRollout) != SkillRolloutNewSession
}

// SkillImageConfig is the pointer to the snapshot that carries the skills
// installed on this sandbox config. Snapshot IDs double as template IDs, so
// nothing else is needed to boot sessions from it.
//
// It holds no secrets, so it is not encrypted by TenantSandboxConfig.Value.
type SkillImageConfig struct {
	// SnapshotID is the currently effective snapshot; empty = base template.
	SnapshotID string `json:"snapshot_id,omitempty"`
	// Generation increments on every successful install/remove, for naming
	// and troubleshooting.
	Generation int `json:"generation,omitempty"`
	// BuiltAt records when this generation was produced.
	BuiltAt time.Time `json:"built_at,omitempty"`
	// BaseTemplateID is the template this chain was originally built from;
	// the rebuild path starts over from it.
	BaseTemplateID string `json:"base_template_id,omitempty"`
	// OwnerFingerprint identifies the provider account that owns the snapshot.
	// Snapshots are invisible across accounts, so a mismatch means "fall back
	// to the base template" rather than "fail".
	OwnerFingerprint string `json:"owner_fingerprint,omitempty"`
}

// Value implements the driver.Valuer interface. Every secret-bearing field
// (Cube.APIKey, E2B.APIKey and all EnvVars values) is encrypted before
// persisting. EnvVars are included because environment variables routinely
// carry credentials, and their values are handed to tenant scripts verbatim.
// The receiver is never mutated: nested structs and the map are copied first.
func (c *TenantSandboxConfig) Value() (driver.Value, error) {
	if c == nil {
		return nil, nil
	}
	cp := *c
	key := utils.GetAESKey()

	encrypt := func(plain string) string {
		if plain == "" || key == nil {
			return plain
		}
		encrypted, err := utils.EncryptAESGCM(plain, key)
		if err != nil {
			return plain
		}
		return encrypted
	}

	if c.Cube != nil {
		cube := *c.Cube
		cube.APIKey = encrypt(cube.APIKey)
		cp.Cube = &cube
	}
	if c.E2B != nil {
		e2b := *c.E2B
		e2b.APIKey = encrypt(e2b.APIKey)
		cp.E2B = &e2b
	}
	// Keys stay readable so operators can still see which variables are set.
	if len(c.EnvVars) > 0 {
		envVars := make(map[string]string, len(c.EnvVars))
		for name, value := range c.EnvVars {
			envVars[name] = encrypt(value)
		}
		cp.EnvVars = envVars
	}

	return json.Marshal(&cp)
}

// Scan implements the sql.Scanner interface. Secrets that cannot be decrypted
// (missing or rotated SYSTEM_AES_KEY) are blanked and logged rather than
// failing the whole load, matching ModelParameters.Scan — a tenant must stay
// listable even if its sandbox credentials became unreadable.
func (c *TenantSandboxConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	if err := json.Unmarshal(b, c); err != nil {
		return err
	}

	decrypt := func(stored, label string) string {
		if stored == "" {
			return ""
		}
		if plain, ok := utils.DecryptStoredSecretLenient(stored); ok {
			return plain
		}
		log.Printf("[crypto] tenant_sandbox_config.%s: decrypt failed "+
			"(SYSTEM_AES_KEY missing/rotated?), treating as unconfigured", label)
		return ""
	}

	if c.Cube != nil {
		c.Cube.APIKey = decrypt(c.Cube.APIKey, "cube.api_key")
	}
	if c.E2B != nil {
		c.E2B.APIKey = decrypt(c.E2B.APIKey, "e2b.api_key")
	}
	for name, stored := range c.EnvVars {
		c.EnvVars[name] = decrypt(stored, "env_vars."+name)
	}

	return nil
}
