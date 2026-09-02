package types

import (
	"database/sql/driver"
	"encoding/json"
)

// StorageEngineConfig holds tenant-level storage engine parameters for Local, MinIO, COS, TOS, S3, OSS, KS3, and OBS.
// Knowledge bases select which provider to use; parameters are read from here.
type StorageEngineConfig struct {
	DefaultProvider string             `json:"default_provider"` // "local", "minio", "cos", "tos", "s3", "oss", "ks3", "obs"
	Local           *LocalEngineConfig `json:"local,omitempty"`
	MinIO           *MinIOEngineConfig `json:"minio,omitempty"`
	COS             *COSEngineConfig   `json:"cos,omitempty"`
	TOS             *TOSEngineConfig   `json:"tos,omitempty"`
	S3              *S3EngineConfig    `json:"s3,omitempty"`
	OSS             *OSSEngineConfig   `json:"oss,omitempty"`
	KS3             *KS3EngineConfig   `json:"ks3,omitempty"`
	OBS             *OBSEngineConfig   `json:"obs,omitempty"`
}

// LocalEngineConfig is for local file system storage (single-machine deployment only).
type LocalEngineConfig struct {
	PathPrefix string `json:"path_prefix"`
}

// MinIOEngineConfig is for MinIO/S3-compatible object storage.
// Mode "docker" uses env vars for endpoint/credentials; "remote" uses the fields below.
type MinIOEngineConfig struct {
	Mode            string `json:"mode"` // "docker" or "remote"
	Endpoint        string `json:"endpoint"`
	AccessKeyID     string `json:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key"`
	BucketName      string `json:"bucket_name"`
	UseSSL          bool   `json:"use_ssl"`
	PathPrefix      string `json:"path_prefix"`
}

// COSEngineConfig is for Tencent Cloud COS.
type COSEngineConfig struct {
	SecretID       string `json:"secret_id"`
	SecretKey      string `json:"secret_key"`
	Region         string `json:"region"`
	BucketName     string `json:"bucket_name"`
	AppID          string `json:"app_id"`
	PathPrefix     string `json:"path_prefix"`
	TempBucketName string `json:"temp_bucket_name"`
	TempRegion     string `json:"temp_region"`
}

// TOSEngineConfig is for Volcengine TOS (火山引擎对象存储).
type TOSEngineConfig struct {
	Endpoint       string `json:"endpoint"`
	Region         string `json:"region"`
	AccessKey      string `json:"access_key"`
	SecretKey      string `json:"secret_key"`
	BucketName     string `json:"bucket_name"`
	PathPrefix     string `json:"path_prefix"`
	TempBucketName string `json:"temp_bucket_name"`
	TempRegion     string `json:"temp_region"`
}

// S3EngineConfig is for AWS S3 and S3-compatible object storage.
type S3EngineConfig struct {
	Endpoint       string `json:"endpoint"`
	Region         string `json:"region"`
	AccessKey      string `json:"access_key"`
	SecretKey      string `json:"secret_key"`
	BucketName     string `json:"bucket_name"`
	PathPrefix     string `json:"path_prefix"`
	UseSSL         bool   `json:"use_ssl"`
	ForcePathStyle bool   `json:"force_path_style"`
}

// OSSEngineConfig is for Alibaba Cloud OSS (对象存储服务).
type OSSEngineConfig struct {
	Endpoint       string `json:"endpoint"`
	Region         string `json:"region"`
	AccessKey      string `json:"access_key"`
	SecretKey      string `json:"secret_key"`
	BucketName     string `json:"bucket_name"`
	PathPrefix     string `json:"path_prefix"`
	UseTempBucket  bool   `json:"use_temp_bucket"`
	TempBucketName string `json:"temp_bucket_name"`
	TempRegion     string `json:"temp_region"`
}

// KS3EngineConfig is for Kingsoft Cloud KS3 object storage.
type KS3EngineConfig struct {
	Endpoint   string `json:"endpoint"`
	Region     string `json:"region"`
	AccessKey  string `json:"access_key"`
	SecretKey  string `json:"secret_key"`
	BucketName string `json:"bucket_name"`
	PathPrefix string `json:"path_prefix"`
}

// OBSEngineConfig is for Huawei Cloud OBS (对象存储服务).
type OBSEngineConfig struct {
	Endpoint   string `json:"endpoint"`
	Region     string `json:"region"`
	AccessKey  string `json:"access_key"`
	SecretKey  string `json:"secret_key"`
	BucketName string `json:"bucket_name"`
	PathPrefix string `json:"path_prefix"`
	UseSSL     bool   `json:"use_ssl"`
}

// Value implements the driver.Valuer interface for StorageEngineConfig
func (c *StorageEngineConfig) Value() (driver.Value, error) {
	if c == nil {
		return nil, nil
	}
	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface for StorageEngineConfig
func (c *StorageEngineConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, c)
}
