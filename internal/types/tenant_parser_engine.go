package types

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"strings"
)

// ParserEngineConfig holds tenant-level overrides for document parser engines (e.g. MinerU endpoint, API key).
// These values take precedence over environment variables when parsing documents.
type ParserEngineConfig struct {
	// ChatParserEngineRules selects parser engines for session-scoped chat
	// documents. Knowledge bases keep their own rules in ChunkingConfig.
	ChatParserEngineRules []ParserEngineRule `json:"chat_parser_engine_rules,omitempty"`
	MinerUEndpoint        string             `json:"mineru_endpoint"` // MinerU 自建服务端点
	MinerUAPIKey          string             `json:"mineru_api_key"`  // MinerU 云 API Key

	// MinerU 自建解析参数
	MinerUModel         string `json:"mineru_model,omitempty"`          // backend: pipeline, vlm-*, hybrid-*
	MinerUVLMServerURL  string `json:"mineru_vlm_server_url,omitempty"` // vLLM 服务器地址 (vlm-http-client / hybrid-http-client)
	MinerUEnableFormula *bool  `json:"mineru_enable_formula,omitempty"`
	MinerUEnableTable   *bool  `json:"mineru_enable_table,omitempty"`
	MinerUParseMethod   string `json:"mineru_parse_method,omitempty"`
	// MinerUEnableOCR is retained for compatibility with configurations saved
	// before parse_method supported auto/ocr/txt.
	MinerUEnableOCR *bool  `json:"mineru_enable_ocr,omitempty"`
	MinerULanguage  string `json:"mineru_language,omitempty"`

	// MinerU 云 API 解析参数
	MinerUCloudModel         string `json:"mineru_cloud_model,omitempty"` // model_version: pipeline, vlm, MinerU-HTML
	MinerUCloudEnableFormula *bool  `json:"mineru_cloud_enable_formula,omitempty"`
	MinerUCloudEnableTable   *bool  `json:"mineru_cloud_enable_table,omitempty"`
	MinerUCloudEnableOCR     *bool  `json:"mineru_cloud_enable_ocr,omitempty"`
	MinerUCloudLanguage      string `json:"mineru_cloud_language,omitempty"`

	// OpenDataLoader PDF (docreader engine); hybrid requires opendataloader-pdf-hybrid service.
	ODLHybrid           string `json:"odl_hybrid,omitempty"`      // off (default), docling-fast, hancom-ai
	ODLHybridURL        string `json:"odl_hybrid_url,omitempty"`  // e.g. http://odl-hybrid:5002
	ODLHybridMode       string `json:"odl_hybrid_mode,omitempty"` // auto, full
	ODLHybridFallback   *bool  `json:"odl_hybrid_fallback,omitempty"`
	ODLMarkdownWithHTML *bool  `json:"odl_markdown_with_html,omitempty"`

	// PaddleOCR-VL self-hosted pipeline service (full /layout-parsing API).
	PaddleOCRVLEndpoint            string `json:"paddleocr_vl_endpoint,omitempty"` // e.g. http://paddleocr-vl:8080
	PaddleOCRVLUseSealRecognition  *bool  `json:"paddleocr_vl_use_seal_recognition,omitempty"`
	PaddleOCRVLUseChartRecognition *bool  `json:"paddleocr_vl_use_chart_recognition,omitempty"`

	// PaddleOCR-VL AI Studio cloud API.
	PaddleOCRVLCloudToken               string `json:"paddleocr_vl_cloud_token,omitempty"`
	PaddleOCRVLCloudModel               string `json:"paddleocr_vl_cloud_model,omitempty"` // e.g. PaddleOCR-VL-1.6
	PaddleOCRVLCloudUseSealRecognition  *bool  `json:"paddleocr_vl_cloud_use_seal_recognition,omitempty"`
	PaddleOCRVLCloudUseChartRecognition *bool  `json:"paddleocr_vl_cloud_use_chart_recognition,omitempty"`
}

const (
	MinerUParseMethodAuto = "auto"
	MinerUParseMethodOCR  = "ocr"
	MinerUParseMethodText = "txt"
)

// ResolveMinerUParseMethod normalizes the explicit MinerU parse method and
// maps the legacy OCR toggle to the closest safe behavior. The old enabled
// value maps to auto instead of ocr so digital PDFs keep their native text
// layer while scanned PDFs are still detected and OCRed by MinerU.
func ResolveMinerUParseMethod(method string, legacyOCREnabled *bool) string {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case MinerUParseMethodAuto:
		return MinerUParseMethodAuto
	case MinerUParseMethodOCR:
		return MinerUParseMethodOCR
	case MinerUParseMethodText:
		return MinerUParseMethodText
	}

	if legacyOCREnabled != nil && !*legacyOCREnabled {
		return MinerUParseMethodText
	}
	return MinerUParseMethodAuto
}

func (c *ParserEngineConfig) ResolveChatParserEngine(fileType string) string {
	if c == nil {
		return ""
	}
	fileType = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(fileType)), ".")
	for _, rule := range c.ChatParserEngineRules {
		for _, candidate := range rule.FileTypes {
			if strings.TrimPrefix(strings.ToLower(strings.TrimSpace(candidate)), ".") == fileType {
				return strings.TrimSpace(rule.Engine)
			}
		}
	}
	return ""
}

// ToOverridesMap returns a map suitable for ParserEngineOverrides in parse requests.
// Keys are snake_case (mineru_endpoint, mineru_api_key, etc.).
func (c *ParserEngineConfig) ToOverridesMap() map[string]string {
	if c == nil {
		return nil
	}
	m := make(map[string]string)
	if c.MinerUEndpoint != "" {
		m["mineru_endpoint"] = c.MinerUEndpoint
	}
	if c.MinerUAPIKey != "" {
		m["mineru_api_key"] = c.MinerUAPIKey
	}
	if c.MinerUModel != "" {
		m["mineru_model"] = c.MinerUModel
	}
	if c.MinerUVLMServerURL != "" {
		m["mineru_vlm_server_url"] = c.MinerUVLMServerURL
	}
	if c.MinerUEnableFormula != nil {
		m["mineru_enable_formula"] = fmt.Sprintf("%v", *c.MinerUEnableFormula)
	}
	if c.MinerUEnableTable != nil {
		m["mineru_enable_table"] = fmt.Sprintf("%v", *c.MinerUEnableTable)
	}
	if c.MinerUParseMethod != "" || c.MinerUEnableOCR != nil {
		m["mineru_parse_method"] = ResolveMinerUParseMethod(c.MinerUParseMethod, c.MinerUEnableOCR)
	}
	if c.MinerUEnableOCR != nil {
		m["mineru_enable_ocr"] = fmt.Sprintf("%v", *c.MinerUEnableOCR)
	}
	if c.MinerULanguage != "" {
		m["mineru_language"] = c.MinerULanguage
	}
	if c.MinerUCloudModel != "" {
		m["mineru_cloud_model"] = c.MinerUCloudModel
	}
	if c.MinerUCloudEnableFormula != nil {
		m["mineru_cloud_enable_formula"] = fmt.Sprintf("%v", *c.MinerUCloudEnableFormula)
	}
	if c.MinerUCloudEnableTable != nil {
		m["mineru_cloud_enable_table"] = fmt.Sprintf("%v", *c.MinerUCloudEnableTable)
	}
	if c.MinerUCloudEnableOCR != nil {
		m["mineru_cloud_enable_ocr"] = fmt.Sprintf("%v", *c.MinerUCloudEnableOCR)
	}
	if c.MinerUCloudLanguage != "" {
		m["mineru_cloud_language"] = c.MinerUCloudLanguage
	}
	if c.ODLHybrid != "" {
		m["odl_hybrid"] = c.ODLHybrid
	}
	if c.ODLHybridURL != "" {
		m["odl_hybrid_url"] = c.ODLHybridURL
	}
	if c.ODLHybridMode != "" {
		m["odl_hybrid_mode"] = c.ODLHybridMode
	}
	if c.ODLHybridFallback != nil {
		m["odl_hybrid_fallback"] = fmt.Sprintf("%v", *c.ODLHybridFallback)
	}
	if c.ODLMarkdownWithHTML != nil {
		m["odl_markdown_with_html"] = fmt.Sprintf("%v", *c.ODLMarkdownWithHTML)
	}
	if c.PaddleOCRVLEndpoint != "" {
		m["paddleocr_vl_endpoint"] = c.PaddleOCRVLEndpoint
	}
	if c.PaddleOCRVLUseSealRecognition != nil {
		m["paddleocr_vl_use_seal_recognition"] = fmt.Sprintf("%v", *c.PaddleOCRVLUseSealRecognition)
	}
	if c.PaddleOCRVLUseChartRecognition != nil {
		m["paddleocr_vl_use_chart_recognition"] = fmt.Sprintf("%v", *c.PaddleOCRVLUseChartRecognition)
	}
	if c.PaddleOCRVLCloudToken != "" {
		m["paddleocr_vl_cloud_token"] = c.PaddleOCRVLCloudToken
	}
	if c.PaddleOCRVLCloudModel != "" {
		m["paddleocr_vl_cloud_model"] = c.PaddleOCRVLCloudModel
	}
	if c.PaddleOCRVLCloudUseSealRecognition != nil {
		m["paddleocr_vl_cloud_use_seal_recognition"] = fmt.Sprintf("%v", *c.PaddleOCRVLCloudUseSealRecognition)
	}
	if c.PaddleOCRVLCloudUseChartRecognition != nil {
		m["paddleocr_vl_cloud_use_chart_recognition"] = fmt.Sprintf("%v", *c.PaddleOCRVLCloudUseChartRecognition)
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// Value implements the driver.Valuer interface for ParserEngineConfig
func (c *ParserEngineConfig) Value() (driver.Value, error) {
	if c == nil {
		return nil, nil
	}
	return json.Marshal(c)
}

// Scan implements the sql.Scanner interface for ParserEngineConfig
func (c *ParserEngineConfig) Scan(value interface{}) error {
	if value == nil {
		return nil
	}
	b, ok := value.([]byte)
	if !ok {
		return nil
	}
	return json.Unmarshal(b, c)
}
