// Package media implements the upload pipeline (ARCHITECTURE.md §2.5):
// presign -> client PUT -> complete -> worker re-encode -> ready.
package media

import "fmt"

// URL renders the public media URL. The scheme is a permanent contract:
// https://media.{domain}/{variant}/{media_id}.webp — the DB stores ids,
// never URLs, so moving the host behind a CDN is a DNS/config change.
func URL(base, variant string, id int64) string {
	return fmt.Sprintf("%s/%s/%d.webp", base, variant, id)
}

// uploadKey is where the client PUTs the original bytes.
func uploadKey(id int64) string { return fmt.Sprintf("orig/%d", id) }

// variantKey is where the worker writes re-encoded WebP variants.
func variantKey(variant string, id int64) string {
	return fmt.Sprintf("%s/%d.webp", variant, id)
}
