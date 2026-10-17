# ADR-0001: Semantic vectors come from the client

## Context

Release binaries and the Docker image are built with `CGO_ENABLED=0`, and the
ONNX embedder needs CGO, the model files (~470 MB) and about 1 GB of RAM per
server. In practice every live server runs the hash embedder, so remote
"semantic" search ranks by hashed tokens. The client (v0.20+) already runs a
correct ONNX model and tokenizer on the developer machine.

## Decision

The server stores vectors that clients send, each tagged with its
`semantic_space_id`, and compares vectors only within one space. A query vector
also comes from the client. The server's own embedder (hash or a hosted API)
stays optional and has a space of its own. The server build stays CGO-free.

## Consequences

- No model download or extra memory on the server.
- Semantic coverage depends on clients at v0.20 or later backfilling vectors;
  memories without a vector in the query's space are ranked by BM25 only.
- Hosts that call the remote MCP directly (Claude, ChatGPT, Codex, Cursor) send
  no vectors, so their searches are BM25 unless the instance configures a
  server-side embedder for its own space.
