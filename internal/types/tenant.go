package types

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/utils"
	"gorm.io/gorm"
)

// retrieverEngineMapping maps RETRIEVE_DRIVER values to retriever engine configurations
var retrieverEngineMapping = map[string][]RetrieverEngineParams{
	"postgres": {
		{RetrieverType: KeywordsRetrieverType, RetrieverEngineType: PostgresRetrieverEngineType},
		{RetrieverType: VectorRetrieverType, RetrieverEngineType: PostgresRetrieverEngineType},
	},
	"elasticsearch_v7": {
		{RetrieverType: KeywordsRetrieverType, RetrieverEngineType: ElasticsearchRetrieverEngineType},
	},
	"elasticsearch_v8": {
		{RetrieverType: KeywordsRetrieverType, RetrieverEngineType: ElasticsearchRetrieverEngineType},
		{RetrieverType: VectorRetrieverType, RetrieverEngineType: ElasticsearchRetrieverEngineType},
	},
	"qdrant": {
		{RetrieverType: KeywordsRetrieverType, RetrieverEngineType: QdrantRetrieverEngineType},
		{RetrieverType: VectorRetrieverType, RetrieverEngineType: QdrantRetrieverEngineType},
	},
	"milvus": {
		{RetrieverType: VectorRetrieverType, RetrieverEngineType: MilvusRetrieverEngineType},
		{RetrieverType: KeywordsRetrieverType, RetrieverEngineType: MilvusRetrieverEngineType},
	},
	"weaviate": {
		{RetrieverType: KeywordsRetrieverType, RetrieverEngineType: WeaviateRetrieverEngineType},
		{RetrieverType: VectorRetrieverType, RetrieverEngineType: WeaviateRetrieverEngineType},
	},
	"doris": {
		{RetrieverType: KeywordsRetrieverType, RetrieverEngineType: DorisRetrieverEngineType},
		{RetrieverType: VectorRetrieverType, RetrieverEngineType: DorisRetrieverEngineType},
	},
	"sqlite": {
		{RetrieverType: KeywordsRetrieverType, RetrieverEngineType: SQLiteRetrieverEngineType},
		{RetrieverType: VectorRetrieverType, RetrieverEngineType: SQLiteRetrieverEngineType},
	},
	"tencent_vectordb": {
		{RetrieverType: KeywordsRetrieverType, RetrieverEngineType: TencentVectorDBRetrieverEngineType},
		{RetrieverType: VectorRetrieverType, RetrieverEngineType: TencentVectorDBRetrieverEngineType},
	},
	"opensearch": {
		{RetrieverType: KeywordsRetrieverType, RetrieverEngineType: OpenSearchRetrieverEngineType},
		{RetrieverType: VectorRetrieverType, RetrieverEngineType: OpenSearchRetrieverEngineType},
	},
}

// GetRetrieverEngineMapping returns the retriever engine mapping
// This allows other packages to access the driver capabilities
func GetRetrieverEngineMapping() map[string][]RetrieverEngineParams {
	return retrieverEngineMapping
}

// GetDefaultRetrieverEngines returns the default retriever engines based on RETRIEVE_DRIVER env
func GetDefaultRetrieverEngines() []RetrieverEngineParams {
	result := []RetrieverEngineParams{}
	seen := make(map[string]bool)

	for _, driver := range strings.Split(os.Getenv("RETRIEVE_DRIVER"), ",") {
		driver = strings.TrimSpace(driver)
		if params, ok := retrieverEngineMapping[driver]; ok {
			for _, p := range params {
				key := string(p.RetrieverType) + ":" + string(p.RetrieverEngineType)
				if !seen[key] {
					seen[key] = true
					result = append(result, p)
				}
			}
		}
	}
	return result
}

// Tenant represents the tenant
type Tenant struct {
	// ID
	ID uint64 `yaml:"id"                  json:"id"                  gorm:"primaryKey"`
	// 平台外部租户标识只用于幂等控制面，不进入普通租户响应。
	ExternalRef *string `yaml:"-" json:"-" gorm:"column:external_ref;type:uuid;uniqueIndex"`
	// Name
	Name string `yaml:"name"                json:"name"`
	// Description
	Description string `yaml:"description"         json:"description"`
	// Status
	Status string `yaml:"status"              json:"status"              gorm:"default:'active'"`
	// Retriever engines
	RetrieverEngines RetrieverEngines `yaml:"retriever_engines"   json:"retriever_engines"   gorm:"type:json"`
	// Business
	Business string `yaml:"business"            json:"business"`
	// Storage quota (Bytes), default is 10GB, including vector, original file, text, index, etc.
	StorageQuota int64 `yaml:"storage_quota"       json:"storage_quota"       gorm:"default:10737418240"`
	// Storage used (Bytes)
	StorageUsed int64 `yaml:"storage_used"        json:"storage_used"        gorm:"default:0"`
	// Global Context configuration for this workspace (default for all sessions)
	ContextConfig *ContextConfig `yaml:"context_config"      json:"context_config"      gorm:"type:jsonb"`
	// Global WebSearch configuration for this workspace
	WebSearchConfig *WebSearchConfig `yaml:"web_search_config"   json:"web_search_config"   gorm:"type:jsonb"`
	// Parser engine config overrides (MinerU endpoint, API key, etc.). Used when parsing documents; overrides env.
	ParserEngineConfig *ParserEngineConfig `yaml:"parser_engine_config" json:"parser_engine_config" gorm:"type:jsonb"`
	// Credentials config: third-party provider credentials (e.g. WeKnoraCloud AppID/AppSecret)
	Credentials *CredentialsConfig `yaml:"credentials" json:"credentials" gorm:"type:jsonb"`
	// Storage engine config: parameters for Local, MinIO, COS. Used for document/file storage and docreader.
	StorageEngineConfig *StorageEngineConfig `yaml:"storage_engine_config" json:"storage_engine_config" gorm:"type:jsonb"`
	// DefaultStorageBackendID is the workspace default concrete storage instance.
	DefaultStorageBackendID *string `yaml:"default_storage_backend_id" json:"default_storage_backend_id,omitempty" gorm:"column:default_storage_backend_id;type:varchar(36)"`
	// Chat history config: knowledge base configuration for indexing and searching chat messages via vector search
	ChatHistoryConfig *ChatHistoryConfig `yaml:"chat_history_config" json:"chat_history_config" gorm:"type:jsonb"`
	// Retrieval config: global search/retrieval parameters shared by knowledge search and message search
	RetrievalConfig *RetrievalConfig `yaml:"retrieval_config" json:"retrieval_config" gorm:"type:jsonb"`
	// Memory config: workspace switch for cross-session long-term memory
	MemoryConfig *MemoryConfig `yaml:"memory_config" json:"memory_config" gorm:"type:jsonb"`
	// API principal config: controls how X-API-Key requests map to terminal principals.
	APIPrincipalConfig *APIPrincipalConfig `yaml:"api_principal_config" json:"-" gorm:"type:jsonb"`
	// Creation time
	CreatedAt time.Time `yaml:"created_at"          json:"created_at"`
	// Last updated time
	UpdatedAt time.Time `yaml:"updated_at"          json:"updated_at"`
	// Deletion time
	DeletedAt gorm.DeletedAt `yaml:"deleted_at"          json:"deleted_at"          gorm:"index"`
}

// RetrieverEngines represents the retriever engines for a tenant
type RetrieverEngines struct {
	Engines []RetrieverEngineParams `yaml:"engines" json:"engines" gorm:"type:json"`
}

// GetEffectiveEngines returns the tenant's engines if configured, otherwise returns system defaults
func (t *Tenant) GetEffectiveEngines() []RetrieverEngineParams {
	if len(t.RetrieverEngines.Engines) > 0 {
		return t.RetrieverEngines.Engines
	}
	return GetDefaultRetrieverEngines()
}

// BeforeCreate is a hook function that is called before creating a tenant
func (t *Tenant) BeforeCreate(tx *gorm.DB) error {
	if t.RetrieverEngines.Engines == nil {
		t.RetrieverEngines.Engines = []RetrieverEngineParams{}
	}
	return nil
}

// Value implements the driver.Valuer interface, used to convert RetrieverEngines to database value
func (c RetrieverEngines) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface, used to convert database value to RetrieverEngines.
// It supports both the legacy bare-array format (e.g. [{...}, {...}]) and the current
// object-wrapped format (e.g. {"engines": [{...}, {...}]}).
func (c *RetrieverEngines) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}

	// Try the current object format first: {"engines": [...]}
	if err := json.Unmarshal(b, c); err == nil {
		return nil
	}

	// Fallback: legacy bare-array format: [{...}, {...}]
	var engines []RetrieverEngineParams
	if err := json.Unmarshal(b, &engines); err != nil {
		return fmt.Errorf("retriever_engines: cannot unmarshal as object or array: %w", err)
	}
	c.Engines = engines
	return nil
}

// CredentialsConfig holds third-party provider credentials at the tenant level.
// Stored as a single JSONB column; each provider is a nested object so new
// providers can be added without schema changes.
type CredentialsConfig struct {
	WeKnoraCloud *WeKnoraCloudCredentials `json:"weknoracloud,omitempty"`
}

// WeKnoraCloudCredentials stores WeKnoraCloud AppID and AppSecret.
// AppSecret is AES-256 encrypted before persisting to database.
type WeKnoraCloudCredentials struct {
	AppID     string `json:"app_id"`
	AppSecret string `json:"app_secret"`
}

type APIPrincipalMode string

const (
	APIPrincipalModeTenant      APIPrincipalMode = "tenant"
	APIPrincipalModeDirect      APIPrincipalMode = "direct_header"
	APIPrincipalModeSignedToken APIPrincipalMode = "signed_token"
)

// APIPrincipalConfig controls how tenant API-key requests map to terminal
// principals. Direct header mode is low-assurance and should only be used for
// trusted server-to-server calls; signed-token mode verifies the user claim.
type APIPrincipalConfig struct {
	Mode                  APIPrincipalMode `json:"mode"`
	DirectHeaderName      string           `json:"direct_header_name,omitempty"`
	SignedTokenHeaderName string           `json:"signed_token_header_name,omitempty"`
	// RequireDirectHeader, when true in direct_header mode, rejects API-key
	// requests that omit the configured user-id header instead of falling
	// back to the tenant-level principal.
	RequireDirectHeader bool   `json:"require_direct_header,omitempty"`
	HMACSecret          string `json:"hmac_secret,omitempty"`
}

func (c *APIPrincipalConfig) Value() (driver.Value, error) {
	if c == nil {
		return nil, nil
	}
	cp := *c
	if cp.HMACSecret != "" {
		encrypted, err := utils.EncryptStoredSecret(cp.HMACSecret)
		if err != nil {
			return nil, fmt.Errorf("encrypt tenant.api_principal_config.hmac_secret: %w", err)
		}
		cp.HMACSecret = encrypted
	}
	return json.Marshal(&cp)
}

func (c *APIPrincipalConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	var b []byte
	switch raw := value.(type) {
	case []byte:
		b = raw
	case string:
		b = []byte(raw)
	default:
		return fmt.Errorf("scan tenant.api_principal_config: unsupported value type %T", value)
	}
	if err := json.Unmarshal(b, c); err != nil {
		return err
	}
	plain, err := utils.DecryptEncryptedStoredSecret(c.HMACSecret)
	if err != nil {
		return fmt.Errorf("decrypt tenant.api_principal_config.hmac_secret: %w", err)
	}
	c.HMACSecret = plain
	return nil
}

// GetWeKnoraCloud returns the WeKnoraCloud credentials, or nil if not configured.
func (c *CredentialsConfig) GetWeKnoraCloud() *WeKnoraCloudCredentials {
	if c == nil || c.WeKnoraCloud == nil {
		return nil
	}
	if c.WeKnoraCloud.AppID == "" || c.WeKnoraCloud.AppSecret == "" {
		return nil
	}
	return c.WeKnoraCloud
}

// Value implements the driver.Valuer interface for CredentialsConfig
func (c *CredentialsConfig) Value() (driver.Value, error) {
	if c == nil {
		return nil, nil
	}
	cp := *c
	if cp.WeKnoraCloud != nil && cp.WeKnoraCloud.AppSecret != "" {
		if key := utils.GetAESKey(); key != nil {
			if encrypted, err := utils.EncryptAESGCM(cp.WeKnoraCloud.AppSecret, key); err == nil {
				cp.WeKnoraCloud = &WeKnoraCloudCredentials{AppID: cp.WeKnoraCloud.AppID, AppSecret: encrypted}
			}
		}
	}
	return json.Marshal(cp)
}

// Scan implements the sql.Scanner interface for CredentialsConfig
func (c *CredentialsConfig) Scan(value interface{}) error {
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
	if c.WeKnoraCloud != nil {
		if plain, ok := utils.DecryptStoredSecretLenient(c.WeKnoraCloud.AppSecret); ok {
			c.WeKnoraCloud.AppSecret = plain
		} else {
			log.Printf("[crypto] tenant credentials we_knora_cloud.app_secret: decrypt failed (SYSTEM_AES_KEY missing/rotated?), treating as unconfigured")
			c.WeKnoraCloud.AppSecret = ""
		}
	}
	return nil
}
