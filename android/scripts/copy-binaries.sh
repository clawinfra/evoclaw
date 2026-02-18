#!/bin/bash
# Copy compiled Rust binaries into Android jniLibs structure
# Run from repo root: ./android/scripts/copy-binaries.sh
set -e

EDGE_AGENT_DIR="edge-agent"
ANDROID_JNI_DIR="android/app/src/main/jniLibs"

echo "📦 Copying EvoClaw native binaries into APK jniLibs..."

declare -A ABI_MAP=(
    ["aarch64-linux-android"]="arm64-v8a"
    ["armv7-linux-androideabi"]="armeabi-v7a"
    ["x86_64-linux-android"]="x86_64"
)

for target in "${!ABI_MAP[@]}"; do
    abi="${ABI_MAP[$target]}"
    src="${EDGE_AGENT_DIR}/target/${target}/release/evoclaw-agent"
    # Try alternate binary name
    if [ ! -f "$src" ]; then
        src="${EDGE_AGENT_DIR}/target/${target}/release/evoclaw_agent"
    fi

    dest_dir="${ANDROID_JNI_DIR}/${abi}"
    dest="${dest_dir}/libevoclaw_agent.so"

    mkdir -p "$dest_dir"

    if [ -f "$src" ]; then
        cp "$src" "$dest"
        echo "  ✅ ${target} → ${abi}/libevoclaw_agent.so ($(du -sh "$dest" | cut -f1))"
    else
        echo "  ⚠️  ${target}: binary not found at ${src} — skipping"
        echo "     Run: cd edge-agent && cargo build --release --target ${target}"
    fi
done

echo ""
echo "Done. jniLibs contents:"
find "$ANDROID_JNI_DIR" -name "*.so" | sort | while read f; do
    echo "  $(du -sh "$f" | cut -f1)  $f"
done
