package types

import (
	"database/sql/driver"
	"encoding/json"
	"time"
)

// MessageExecutionContext is a message-level snapshot of the non-secret
// request state used by derived experiences such as follow-up suggestions.
type MessageExecutionContext struct {
	AgentConfigHash       string                    `json:"agent_config_hash,omitempty"`
	QuestionSuggestions   *QuestionSuggestionConfig `json:"question_suggestions,omitempty"`
	KnowledgeBaseIDs      []string                  `json:"knowledge_base_ids,omitempty"`
	KnowledgeIDs          []string                  `json:"knowledge_ids,omitempty"`
	TagIDs                []string                  `json:"tag_ids,omitempty"`
	TagScopes             []TagScope                `json:"tag_scopes,omitempty"`
	MCPServiceIDs         []string                  `json:"mcp_service_ids,omitempty"`
	SkillNames            []string                  `json:"skill_names,omitempty"`
	WebSearchEnabled      bool                      `json:"web_search_enabled"`
	Locale                string                    `json:"locale,omitempty"`
	SuggestionAttribution *SuggestionAttribution    `json:"suggestion_attribution,omitempty"`
}

func (c MessageExecutionContext) Value() (driver.Value, error) {
	return json.Marshal(c)
}

func (c *MessageExecutionContext) Scan(value interface{}) error {
	if value == nil {
		*c = MessageExecutionContext{}
		return nil
	}
	var b []byte
	switch v := value.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		*c = MessageExecutionContext{}
		return nil
	}
	return json.Unmarshal(b, c)
}

// AgentSteps represents a collection of agent execution steps
// Used for storing agent reasoning process in database
type AgentSteps []AgentStep

// Value implements the driver.Valuer interface for database serialization
func (a AgentSteps) Value() (driver.Value, error) {
	if a == nil {
		return json.Marshal([]AgentStep{})
	}
	return json.Marshal(a)
}

// Scan implements the sql.Scanner interface for database deserialization
func (a *AgentSteps) Scan(value interface{}) error {
	if value == nil {
		*a = make(AgentSteps, 0)
		return nil
	}
	var b []byte
	switch v := value.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		*a = make(AgentSteps, 0)
		return nil
	}
	return json.Unmarshal(b, a)
}

// MessageSearchMode represents the search mode for message search
type MessageSearchMode string

const (
	// MessageSearchModeKeyword searches by keyword only
	MessageSearchModeKeyword MessageSearchMode = "keyword"
	// MessageSearchModeVector searches by vector similarity only
	MessageSearchModeVector MessageSearchMode = "vector"
	// MessageSearchModeHybrid combines keyword and vector search with RRF fusion
	MessageSearchModeHybrid MessageSearchMode = "hybrid"
)

// MessageSearchParams defines the parameters for searching chat history messages
type MessageSearchParams struct {
	// Query text for search
	Query string `json:"query" binding:"required"`
	// Search mode: "keyword", "vector", "hybrid" (default: "hybrid")
	Mode MessageSearchMode `json:"mode"`
	// Maximum number of results to return (default: 20)
	Limit int `json:"limit"`
	// Filter by specific session IDs (optional, empty means all sessions)
	SessionIDs []string `json:"session_ids"`
	// OwnerID restricts results to sessions belonging to one person. It is set
	// from the caller's identity rather than from the request body: conversation
	// search must not be a way to read a colleague's private chats.
	OwnerID string `json:"-"`
}

// MessageWithSession extends Message with session title for search results
type MessageWithSession struct {
	Message
	// Title of the session this message belongs to
	SessionTitle string `json:"session_title"`
}

// MessageSearchResultItem represents a single search result item (internal, pre-merge)
type MessageSearchResultItem struct {
	// The matched message with session info
	MessageWithSession
	// Search relevance score (higher is better)
	Score float64 `json:"score"`
	// How this result was matched: "keyword", "vector", or "hybrid"
	MatchType string `json:"match_type"`
}

// MessageSearchGroupItem represents a merged Q&A pair in search results.
// Messages sharing the same request_id are grouped together so that the user query
// and assistant answer are displayed side by side.
type MessageSearchGroupItem struct {
	// The request_id that groups Q&A together
	RequestID string `json:"request_id"`
	// Session info
	SessionID    string `json:"session_id"`
	SessionTitle string `json:"session_title"`
	// User query content (role=user)
	QueryContent string `json:"query_content"`
	// Assistant answer content (role=assistant), may be empty if only Q matched
	AnswerContent string `json:"answer_content"`
	// Best score among the matched messages in this group
	Score float64 `json:"score"`
	// How this result was matched: "keyword", "vector", or "hybrid"
	MatchType string `json:"match_type"`
	// Timestamp of the earliest message in the group
	CreatedAt time.Time `json:"created_at"`
}

// MessageSearchResult represents the search result for message search
type MessageSearchResult struct {
	// List of merged Q&A pairs
	Items []*MessageSearchGroupItem `json:"items"`
	// Total number of results
	Total int `json:"total"`
}

// ChatHistoryKBStats represents statistics about the chat history knowledge base
type ChatHistoryKBStats struct {
	// Whether the chat history KB is configured and enabled
	Enabled bool `json:"enabled"`
	// ID of the embedding model used
	EmbeddingModelID string `json:"embedding_model_id,omitempty"`
	// ID of the knowledge base used for chat history
	KnowledgeBaseID string `json:"knowledge_base_id,omitempty"`
	// Name of the knowledge base
	KnowledgeBaseName string `json:"knowledge_base_name,omitempty"`
	// Number of indexed message entries (Knowledge count)
	IndexedMessageCount int64 `json:"indexed_message_count"`
	// Whether there are any indexed messages (used by frontend to lock embedding model)
	HasIndexedMessages bool `json:"has_indexed_messages"`
}
