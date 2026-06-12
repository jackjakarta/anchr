1. Fuzzy filter / search within a listing — There's no way to filter the current view. A / key that
  opens a filter input and narrows b.items live would be huge for buckets with hundreds of keys. Low
  effort, high payoff for a keyboard-driven tool.
  
  2. Object preview pane — D downloads, but you can't peek. Add a preview (e.g. space or p) that streams
  the first N KB via a ranged GetObject and renders text/JSON/images-as-metadata in a popup or
  right-split. This is the killer feature for a TUI browser.
  
  3. Sorting — renderItem already shows NAME/SIZE/DATE columns but you can't sort by them. Add s to
  cycle sort key (name → size → modified) and reverse. Pure local operation on b.items.

  4. Copy key / copy S3 URI / generate presigned URL — y to yank the selected object's key or
  s3://bucket/key to clipboard, and a key to mint a presigned GET URL (aws-sdk-go-v2 has
  s3.NewPresignClient). Extremely handy day-to-day. -> Done ✅
  
  Correctness issue worth treating as a feature

  5. Pagination — this is actually a latent bug. ListObjects (client.go:64) sets MaxKeys: 1000 and never
  checks IsTruncated / NextContinuationToken. Any prefix with >1000 objects/folders is silently 
  truncated. Fix by either looping the paginator (s3.NewListObjectsV2Paginator) or lazy-loading more on
  scroll. I'd prioritize this — it's a quiet data-correctness problem. -> Done ✅

  Bigger features

  6. Cross-platform downloads. CLAUDE.md flags it: chooseDownloadDest shells out to osascript, so
  downloads are macOS-only. Replace with a Bubble Tea text-input prompt for the destination path (works
  everywhere) and keep the native panel as a macOS nicety. This unblocks Linux/Windows users entirely.
  
  7. Upload + delete + mkdir. Currently read-only-ish (download only). Real bucket management — u to
  upload a local file, x/dd to delete (with a confirm modal), create-folder — turns this from a viewer
  into a tool.
  
  8. Recursive / folder download. Download a whole prefix as a tree, with a progress bar. Pairs
  naturally with the cross-platform download work.

  9. Multi-select. A tea.Model-friendly selection set so you can mark several objects (space) and
  download/delete in bulk.

  Polish

  - Breadcrumb nav + jump-to-key — the header (browser.go:150) shows the path as a string; make segments
  selectable or add a g "go to key/prefix" prompt.
  - Object detail view — full metadata: storage class, ETag, content-type, exact bytes, full timestamp.
  - Bucket search in sidebar — once you have many buckets configured.
  - Config-free / discovery mode — aws s3 ls-style: list all buckets from creds instead of requiring
  each in config.yaml.
  - Tests — there are none (go vet only). Even a few table tests around prefix/../ index math
  (selectedItem, renderItem) and formatSize would protect the trickiest logic.