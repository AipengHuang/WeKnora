package service

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/Tencent/WeKnora/internal/logger"
	"github.com/Tencent/WeKnora/internal/sandbox"
	"github.com/Tencent/WeKnora/internal/types"
	"github.com/google/uuid"
)

// loadKnownSet returns the (source_path, mod_time) tuples already recorded
// against the session. Empty on error so the caller can proceed.
func (c *ArtifactCollector) loadKnownSet(ctx context.Context, sessionID string) map[string]struct{} {
	set := map[string]struct{}{}
	if c.store == nil {
		return set
	}
	prev, err := c.store.KnownArtifacts(ctx, sessionID)
	if err != nil {
		logger.Warnf(ctx, "[ArtifactCollector] load previous artifacts failed: session=%s err=%v",
			sessionID, err)
		return set
	}
	for _, p := range prev {
		set[artifactKey(p.SourcePath, p.ModTime)] = struct{}{}
	}
	return set
}

func (c *ArtifactCollector) acceptEntry(entry sandbox.RemoteDirEntry, known map[string]struct{}) bool {
	if entry.Type != sandbox.RemoteEntryFile {
		return false
	}
	if entry.Path == "" || entry.Name == "" {
		return false
	}
	if entry.Size > c.config.MaxFileBytes {
		return false
	}
	if _, seen := known[artifactKey(entry.Path, entry.ModTime)]; seen {
		return false
	}
	return true
}

// maybePersist runs the per-file pipeline (filter → download → upload →
// build metadata). Returns ok=false when the entry was skipped for any
// reason (already known, too large, upload failed). All skip reasons are
// logged so operators can diagnose empty artifact panels.
func (c *ArtifactCollector) maybePersist(
	ctx context.Context,
	source SandboxArtifactSource,
	sessionID string,
	messageID string,
	toolCallID string,
	tenantID uint64,
	entry sandbox.RemoteDirEntry,
	known map[string]struct{},
) (types.MessageArtifact, bool) {
	if !c.acceptEntry(entry, known) {
		if entry.Type == sandbox.RemoteEntryFile && entry.Size > c.config.MaxFileBytes {
			logger.Warnf(ctx, "[ArtifactCollector] skip oversize artifact: session=%s path=%s size=%d limit=%d",
				sessionID, entry.Path, entry.Size, c.config.MaxFileBytes)
		}
		return types.MessageArtifact{}, false
	}

	data, err := source.ReadSessionFile(ctx, sessionID, entry.Path)
	if err != nil {
		logger.Warnf(ctx, "[ArtifactCollector] read artifact failed: session=%s path=%s err=%v",
			sessionID, entry.Path, err)
		return types.MessageArtifact{}, false
	}
	// A second guard: envd may report a stale size while the file is being
	// re-written; enforce the cap against the actual byte count too.
	if int64(len(data)) > c.config.MaxFileBytes {
		logger.Warnf(ctx, "[ArtifactCollector] skip oversize artifact after read: session=%s path=%s size=%d limit=%d",
			sessionID, entry.Path, len(data), c.config.MaxFileBytes)
		return types.MessageArtifact{}, false
	}

	// Give each blob a UUID-namespaced storage name so concurrent turns
	// cannot collide, and so the storage key itself is unguessable from
	// the outside (defence-in-depth on top of the /artifacts/:index
	// endpoint's ownership check).
	storageName := "artifact_" + uuid.NewString() + "_" + safeFileName(entry.Name)
	storagePath, err := c.fileService.SaveBytes(ctx, data, tenantID, storageName, false)
	if err != nil {
		logger.Warnf(ctx, "[ArtifactCollector] upload artifact failed: session=%s path=%s err=%v",
			sessionID, entry.Path, err)
		return types.MessageArtifact{}, false
	}

	c.bindArtifactResource(ctx, storagePath, messageID)

	return types.MessageArtifact{
		URL:        storagePath,
		ToolCallID: toolCallID,
		FileName:   entry.Name,
		FileType:   strings.ToLower(filepath.Ext(entry.Name)),
		FileSize:   int64(len(data)),
		SourcePath: entry.Path,
		ModTime:    entry.ModTime,
		CreatedAt:  time.Now().UTC(),
	}, true
}

// bindArtifactResource records that the freshly-persisted artifact resource
// is owned by its assistant message. Best-effort: a binding failure never
// discards the artifact, because the file is already stored and remains
// downloadable through the /artifacts endpoint regardless of the binding.
//
// The binding is only attempted when (a) the catalog is wired in, (b) we have
// a message ID to own the resource, and (c) SaveBytes actually returned a
// resource:// reference — i.e. the file service is resource-catalog-backed.
// Raw provider paths (no catalog decorator) are left unbound rather than
// generating spurious "invalid resource reference" errors.
func (c *ArtifactCollector) bindArtifactResource(ctx context.Context, ref, messageID string) {
	if c.catalog == nil || messageID == "" {
		return
	}
	if _, ok := types.ParseResourcePath(ref); !ok {
		return
	}
	if err := c.catalog.Bind(ctx, ref, artifactBindingOwnerType, messageID, artifactBindingRelation); err != nil {
		logger.Warnf(ctx, "[ArtifactCollector] bind artifact resource failed: message=%s ref=%s err=%v",
			messageID, ref, err)
	}
}

// artifactKey is the string form of the (source_path, mtime) tuple used to
// de-duplicate artifacts across messages. mtime is normalised to UTC + RFC3339
// nano so equality is stable across time-zone or precision differences
// between sandbox envd builds.
func artifactKey(path string, mod time.Time) string {
	if mod.IsZero() {
		return path + "\x00"
	}
	return path + "\x00" + mod.UTC().Format(time.RFC3339Nano)
}

// safeFileName strips slashes and backslashes from the original name before
// concatenating it into the storage key. The FileService may or may not
// sanitise on its own; belt-and-suspenders here avoids provider-specific
// surprises (e.g. object stores that treat "/" as delimiter).
func safeFileName(name string) string {
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	if name == "" {
		return "unnamed"
	}
	return name
}
