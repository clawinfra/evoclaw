# EvoClaw Android App

Android wrapper that runs the native EvoClaw edge agent as a persistent foreground service.

## Architecture

```
EvoClaw APK
├── AgentService.kt       — Foreground service, manages agent process lifecycle
├── BootReceiver.kt       — Auto-starts agent after device reboot
├── MainActivity.kt       — Simple dashboard UI (status + log viewer)
├── jniLibs/
│   ├── arm64-v8a/        ← evoclaw-agent binary (aarch64-linux-android)
│   ├── armeabi-v7a/      ← evoclaw-agent binary (armv7-linux-androideabi)
│   └── x86_64/           ← evoclaw-agent binary (x86_64-linux-android, emulator)
└── agent.toml            ← generated on first launch in app filesDir
```

The Rust binary is bundled as `libevoclaw_agent.so` (Android jniLibs naming convention)
but executed directly as a child process — **not** loaded as a shared library.

## Building

### Prerequisites
- Android Studio Ladybug (2024.2+) or `sdkmanager`
- JDK 17+
- Rust targets (already set up by CI)

### Step 1: Build native binaries (CI does this automatically)
```bash
cd edge-agent
cargo build --release --target aarch64-linux-android
cargo build --release --target armv7-linux-androideabi
cargo build --release --target x86_64-linux-android
```

### Step 2: Copy binaries into APK jniLibs
```bash
./android/scripts/copy-binaries.sh
```

This copies the binaries as `libevoclaw_agent.so` into the correct ABI directories.

### Step 3: Build APK
```bash
cd android
./gradlew assembleDebug    # debug APK
./gradlew assembleRelease  # release APK (needs signing config)
```

Output: `android/app/build/outputs/apk/`

## Installation

### Via ADB (sideload)
```bash
adb install app/build/outputs/apk/debug/app-debug.apk
```

### Via F-Droid (future)
We plan to submit to F-Droid for easy installation without Play Store.

### Via Termux (no APK needed)
For advanced users who prefer Termux:
```bash
curl -fsSL https://raw.githubusercontent.com/clawinfra/evoclaw/main/scripts/install-termux.sh | bash
```

## Configuration

On first launch, a default `agent.toml` is created in the app's private files directory:
```
/data/data/io.clawinfra.evoclaw/files/agent.toml
```

Edit via ADB:
```bash
adb shell run-as io.clawinfra.evoclaw cat /data/data/io.clawinfra.evoclaw/files/agent.toml
adb push my-agent.toml /data/data/io.clawinfra.evoclaw/files/agent.toml
```

Or via the app settings UI (coming in v0.2.0).

## Permissions

| Permission | Purpose |
|-----------|---------|
| INTERNET | MQTT + LLM API |
| FOREGROUND_SERVICE | Keep agent alive in background |
| RECEIVE_BOOT_COMPLETED | Auto-start on reboot |
| WAKE_LOCK | Prevent CPU sleep while processing |
| CAMERA | Optional: camera snapshot capability |
| POST_NOTIFICATIONS | Android 13+ foreground service notification |

## Battery Impact

The agent is designed to be power-efficient:
- MQTT keepalive: 60s interval
- LLM calls only on demand
- No polling loops — pure event-driven
- Estimated: ~2-5% battery/day on modern phones
