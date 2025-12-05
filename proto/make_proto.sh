#!/usr/bin/env bash
# update-protos.sh
# Minimal, Go-focused proto updater.
# - Scans ./proto for .proto files (non-recursive)
# - For each .proto it writes:
#     - a descriptor set file: proto/<name>.pb
#     - Go code: proto/<name>.pb.go and proto/<name>_grpc.pb.go
#
# Usage:
#   ./update-protos.sh        # run from repo root (expects ./proto)
#   ./update-protos.sh -d dir # use a different proto directory
#
# Make executable: chmod +x update-protos.sh
set -euo pipefail

PROTO_DIR="proto"

print_usage() {
  echo "Usage: $0 [-d proto_dir]"
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -d) PROTO_DIR="$2"; shift 2 ;;
    -h|--help) print_usage ;;
    *) echo "Unknown arg: $1"; print_usage ;;
  esac
done

# Require protoc
if ! command -v protoc >/dev/null 2>&1; then
  echo "ERROR: protoc not found in PATH. Install protoc and try again."
  exit 2
fi

# Recommend Go plugins (not strictly required if installed in PATH)
if ! command -v protoc-gen-go >/dev/null 2>&1; then
  echo "WARNING: protoc-gen-go not found in PATH."
  echo "  Install with: go install google.golang.org/protobuf/cmd/protoc-gen-go@latest"
fi
if ! command -v protoc-gen-go-grpc >/dev/null 2>&1; then
  echo "WARNING: protoc-gen-go-grpc not found in PATH."
  echo "  Install with: go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest"
fi

if [[ ! -d "$PROTO_DIR" ]]; then
  echo "ERROR: proto directory '$PROTO_DIR' does not exist."
  exit 3
fi

# Collect .proto files (non-recursive)
mapfile -d '' PROTOS < <(find "$PROTO_DIR" -maxdepth 1 -type f -name '*.proto' -print0)

if [[ ${#PROTOS[@]} -eq 0 ]]; then
  echo "No .proto files found in '$PROTO_DIR'. Nothing to do."
  exit 0
fi

echo "Updating ${#PROTOS[@]} proto file(s) in: $PROTO_DIR"
echo

for p in "${PROTOS[@]}"; do
  # strip trailing NUL (mapfile keeps delimiter out but be defensive)
  p="${p%$'\0'}"
  filename="$(basename -- "$p")"
  base="${filename%.proto}"
  echo "Processing: $filename"

  # write descriptor set to proto/<name>.pb (includes imports + source info)
  desc_out="$PROTO_DIR/${base}.pb"
  echo " - descriptor: $desc_out"
  protoc -I="$PROTO_DIR" --descriptor_set_out="$desc_out" --include_imports --include_source_info "$p"

  # generate Go code (source-relative so files land alongside protos)
  echo " - go: generating ${base}.pb.go and ${base}_grpc.pb.go in $PROTO_DIR"
  protoc -I="$PROTO_DIR" \
    --go_out=paths=source_relative:"$PROTO_DIR" \
    --go-grpc_out=paths=source_relative:"$PROTO_DIR" \
    "$p"

  echo
done

echo "Done. Generated files are in: $PROTO_DIR"