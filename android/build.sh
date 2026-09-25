#!/bin/bash
# Build an API 23 APK with the Ubuntu universe Android packages.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
BUILD="$ROOT/build"
ANDROID_JAR="/usr/lib/android-sdk/platforms/android-23/android.jar"
JAVAC="/usr/lib/jvm/java-17-openjdk-amd64/bin/javac"
JAR="/usr/lib/jvm/java-17-openjdk-amd64/bin/jar"
KEYTOOL="/usr/lib/jvm/java-17-openjdk-amd64/bin/keytool"
FRAMEWORK_RES="/usr/share/android-framework-res/framework-res.apk"

if [[ ! -f "$ANDROID_JAR" ]]; then
  echo "missing $ANDROID_JAR" >&2
  echo "install android-sdk-platform-23" >&2
  exit 1
fi
if [[ ! -x "$JAVAC" ]]; then
  echo "missing $JAVAC" >&2
  echo "install openjdk-17-jdk" >&2
  exit 1
fi

for tool in aapt dalvik-exchange zipalign apksigner; do
  if ! command -v "$tool" >/dev/null; then
    echo "missing $tool" >&2
    exit 1
  fi
done

# aapt resolves android: attributes against a framework resource table.
# The platform android.jar has one when it contains resources.arsc. Otherwise
# the universe package android-framework-res supplies framework-res.apk.
FRAMEWORK="$ANDROID_JAR"
if ! "$JAR" tf "$ANDROID_JAR" | grep -q '^resources.arsc$'; then
  if [[ ! -f "$FRAMEWORK_RES" ]]; then
    echo "android.jar has no resources.arsc and $FRAMEWORK_RES is missing" >&2
    echo "install android-framework-res" >&2
    exit 1
  fi
  FRAMEWORK="$FRAMEWORK_RES"
fi

rm -rf "$BUILD"
mkdir -p "$BUILD/gen" "$BUILD/classes"

aapt package -f \
  -m -J "$BUILD/gen" \
  -M "$ROOT/AndroidManifest.xml" \
  -S "$ROOT/res" \
  -I "$FRAMEWORK" \
  -F "$BUILD/unaligned.apk"

find "$ROOT/src" "$BUILD/gen" -name '*.java' > "$BUILD/sources.list"
"$JAVAC" --release 8 -encoding UTF-8 \
  -classpath "$ANDROID_JAR" \
  -d "$BUILD/classes" \
  @"$BUILD/sources.list"

# dalvik-exchange is Debian's name for the dx tool.
dalvik-exchange --dex --output="$BUILD/classes.dex" "$BUILD/classes"
# The zip entry must be named classes.dex at the archive root.
(
  cd "$BUILD"
  aapt add -f unaligned.apk classes.dex >/dev/null
)

zipalign -f 4 "$BUILD/unaligned.apk" "$BUILD/aligned.apk"

# Keep one debug key so reinstalls keep the same signature.
KS="$ROOT/debug.keystore"
if [[ ! -f "$KS" ]]; then
  "$KEYTOOL" -genkeypair -keystore "$KS" -storepass android -keypass android \
    -alias androiddebugkey -keyalg RSA -keysize 2048 -validity 10000 \
    -dname "CN=Andscreen Debug,O=Andscreen,C=US"
fi

# Android 6 installs signature scheme v1. v2 is included as well.
apksigner sign \
  --ks "$KS" \
  --ks-pass pass:android \
  --key-pass pass:android \
  --ks-key-alias androiddebugkey \
  --v1-signing-enabled true \
  --v2-signing-enabled true \
  --out "$BUILD/andscreen.apk" \
  "$BUILD/aligned.apk"

apksigner verify --verbose --min-sdk-version 23 "$BUILD/andscreen.apk"
echo "built $BUILD/andscreen.apk"
