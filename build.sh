#!/usr/bin/env bash
set -euo pipefail

# Hearsay Build Script (Windows, Linux & macOS)
# Usage:
#   ./build.sh             # Run lint, test, build Windows & Linux binaries, and package into dist/
#   ./build.sh win         # Build Windows GUI executable into build/bin/Hearsay.exe & Hearsay-Setup.exe
#   ./build.sh linux       # Build Linux GUI executable into build/bin/hearsay
#   ./build.sh mac         # Build macOS GUI executable into build/bin/hearsay
#   ./build.sh lint        # Run go fix --diff and golangci-lint-v2
#   ./build.sh test        # Run unit tests with race detector
#   ./build.sh package     # Package zip/tar.gz distributions into dist/
#   ./build.sh package-mac # Package macOS distribution into dist/
#   ./build.sh clean       # Clean build and dist outputs

TARGET="${1:-all}"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BUILD_DIR="$ROOT_DIR/build"
BIN_DIR="$BUILD_DIR/bin"
DIST_DIR="$ROOT_DIR/dist"
DEFAULT_VER=$(grep -E '^[[:space:]]*Version[[:space:]]*=' "$ROOT_DIR/FyneApp.toml" 2>/dev/null | head -n1 | sed -E 's/.*"([^"]+)".*/\1/' || true)
VERSION="${VERSION:-${DEFAULT_VER:-0.0.4}}"

mkdir -p "$BUILD_DIR" "$BIN_DIR" "$DIST_DIR" "$ROOT_DIR/cmd/launcher/payload" "$ROOT_DIR/cmd/installer/payload"
touch "$ROOT_DIR/cmd/launcher/payload/placeholder.txt" "$ROOT_DIR/cmd/installer/payload/placeholder.txt"

if command -v git &>/dev/null && [ -d "$ROOT_DIR/.git" ] && [ -d "$ROOT_DIR/.githooks" ]; then
    git -C "$ROOT_DIR" config core.hooksPath .githooks 2>/dev/null || true
fi

for dir in "$HOME/go/bin" "/usr/local/go/bin" "$HOME/.asdf/shims" "$HOME/.asdf/bin" "/home/linuxbrew/.linuxbrew/bin" "/home/linuxbrew/.linuxbrew/sbin"; do
    if [ -d "$dir" ] && [[ ":$PATH:" != *":$dir:"* ]]; then
        export PATH="$dir:$PATH"
    fi
done

fetch_dlls() {
    echo "==> Verifying Windows runtime DLLs in $BIN_DIR..."
    if [ ! -f "$BIN_DIR/onnxruntime.dll" ] || [ ! -f "$BIN_DIR/sherpa-onnx-c-api.dll" ]; then
        echo "--> Fetching sherpa-onnx Windows x64 DLLs from Go module cache..."
        go mod download github.com/k2-fsa/sherpa-onnx-go-windows 2>/dev/null || true
        local win_mod_dir
        win_mod_dir=$(go list -m -f '{{.Dir}}' github.com/k2-fsa/sherpa-onnx-go-windows 2>/dev/null || true)
        if [ -n "$win_mod_dir" ] && [ -d "$win_mod_dir/lib/x86_64-pc-windows-gnu" ]; then
            cp -f "$win_mod_dir/lib/x86_64-pc-windows-gnu/"*.dll "$BIN_DIR/"
            echo "✓ Copied Windows runtime DLLs from $win_mod_dir to $BIN_DIR"
        elif [ -f "$ROOT_DIR/../TownCrier/build/bin/onnxruntime.dll" ]; then
            cp -f "$ROOT_DIR/../TownCrier/build/bin/"*.dll "$BIN_DIR/"
            echo "✓ Copied Windows runtime DLLs from ../TownCrier/build/bin to $BIN_DIR"
        else
            echo "Warning: Could not locate sherpa-onnx-go-windows module cache."
        fi
    else
        echo "✓ Windows runtime DLLs present."
    fi
}

fetch_linux_libs() {
    echo "==> Verifying Linux runtime shared libraries in $BIN_DIR..."
    if [ ! -f "$BIN_DIR/libonnxruntime.so" ] || [ ! -f "$BIN_DIR/libsherpa-onnx-c-api.so" ]; then
        echo "--> Fetching sherpa-onnx Linux x64 shared libraries from Go module cache..."
        go mod download github.com/k2-fsa/sherpa-onnx-go-linux 2>/dev/null || true
        local linux_mod_dir
        linux_mod_dir=$(go list -m -f '{{.Dir}}' github.com/k2-fsa/sherpa-onnx-go-linux 2>/dev/null || true)
        if [ -n "$linux_mod_dir" ] && [ -d "$linux_mod_dir/lib/x86_64-unknown-linux-gnu" ]; then
            cp -f "$linux_mod_dir/lib/x86_64-unknown-linux-gnu/"*.so "$BIN_DIR/"
            echo "✓ Copied Linux shared libraries from $linux_mod_dir to $BIN_DIR"
        else
            echo "Warning: Could not locate sherpa-onnx-go-linux module cache."
        fi
    else
        echo "✓ Linux runtime shared libraries present."
    fi
}

fetch_mac_libs() {
    echo "==> Verifying macOS runtime shared libraries in $BIN_DIR..."
    if [ ! -f "$BIN_DIR/libonnxruntime.dylib" ] || [ ! -f "$BIN_DIR/libsherpa-onnx-c-api.dylib" ]; then
        echo "--> Fetching sherpa-onnx macOS shared libraries from Go module cache..."
        go mod download github.com/k2-fsa/sherpa-onnx-go-macos 2>/dev/null || true
        local mac_mod_dir
        mac_mod_dir=$(go list -m -f '{{.Dir}}' github.com/k2-fsa/sherpa-onnx-go-macos 2>/dev/null || true)
        local arch
        arch="$(uname -m)"
        local mac_subdir="aarch64-apple-darwin"
        if [ "$arch" = "x86_64" ]; then
            mac_subdir="x86_64-apple-darwin"
        fi
        if [ -n "$mac_mod_dir" ] && [ -d "$mac_mod_dir/lib/$mac_subdir" ]; then
            cp -f "$mac_mod_dir/lib/$mac_subdir/"*.dylib "$BIN_DIR/"
            echo "✓ Copied macOS ($mac_subdir) shared libraries from $mac_mod_dir to $BIN_DIR"
        else
            echo "Warning: Could not locate sherpa-onnx-go-macos module cache for $mac_subdir."
        fi
    else
        echo "✓ macOS runtime shared libraries present."
    fi
}

run_lint() {
    echo "==> Verifying code modernizations (go fix -diff)..."
    local diff_output
    local err_file="$BUILD_DIR/gofix.err"
    if ! diff_output=$(go fix -diff ./... 2>"$err_file"); then
        echo "Error: 'go fix -diff ./...' failed or found unfixed modernizations:"
        if [ -n "$diff_output" ]; then
            echo "$diff_output"
        fi
        cat "$err_file"
        rm -f "$err_file"
        exit 1
    fi
    rm -f "$err_file"
    if [ -n "$diff_output" ]; then
        echo "Error: Unfixed code modernization diffs found:"
        echo "$diff_output"
        echo "Run 'go fix ./...' to apply them."
        exit 1
    fi
    echo "✓ Code modernizations verified (0 diff)!"

    echo "==> Running golangci-lint-v2..."
    if ! command -v golangci-lint-v2 &>/dev/null; then
        echo "Error: golangci-lint-v2 not found in PATH"
        exit 1
    fi
    golangci-lint-v2 run ./...
    echo "✓ Lint passed (0 issues)!"
}

run_test() {
    echo "==> Running unit tests with race detector..."
    go test -race -timeout 180s -count=1 ./...
    echo "==> Running multi-count race stress tests (-count=5) on concurrent listener package..."
    go test -race -timeout 180s -count=5 ./internal/listener/...
    echo "✓ All unit and race tests passed!"
}

generate_winres_syso() {
    local out_prefix="$1"
    local product_name="$2"
    local description="$3"
    local orig_filename="$4"
    local icon_path="$ROOT_DIR/assets/icon.png"
    if [ ! -f "$icon_path" ]; then
        return 0
    fi
    local ver="${VERSION:-0.0.6}"
    ver="${ver#v}"
    local file_ver="${ver%%-*}"
    go tool go-winres simply \
        --arch amd64 \
        --out "$out_prefix" \
        --icon "$icon_path" \
        --manifest gui \
        --product-name "$product_name" \
        --file-description "$description" \
        --product-version "$ver" \
        --file-version "$file_ver" \
        --original-filename "$orig_filename"
}

clean_payload_dir() {
    local dir="$1"
    mkdir -p "$dir"
    find "$dir" -mindepth 1 ! -name 'placeholder.txt' -exec rm -rf {} + 2>/dev/null || true
    touch "$dir/placeholder.txt"
}

pack_payload() {
    local stage_dir="$1"
    local target_dir="$2"
    clean_payload_dir "$target_dir"
    go run github.com/mrpoundsign/appinstaller/cmd/packpayload \
        -src "$stage_dir" \
        -out "$target_dir/payload.tar.zst"
}

build_win() {
    fetch_dlls

    local payload_dir="$ROOT_DIR/cmd/launcher/payload"
    local inst_payload_dir="$ROOT_DIR/cmd/installer/payload"
    local launcher_stage="$BUILD_DIR/launcher_payload_stage_win"
    local inst_stage="$BUILD_DIR/installer_payload_stage_win"
    local core_syso="$ROOT_DIR/cmd/hearsay/rsrc_windows_amd64.syso"
    local launcher_syso="$ROOT_DIR/cmd/launcher/rsrc_windows_amd64.syso"
    local installer_syso="$ROOT_DIR/cmd/installer/rsrc_windows_amd64.syso"
    trap 'rm -rf "$BUILD_DIR/launcher_payload_stage_win" "$BUILD_DIR/installer_payload_stage_win"; clean_payload_dir "$ROOT_DIR/cmd/launcher/payload"; clean_payload_dir "$ROOT_DIR/cmd/installer/payload"; rm -f "$ROOT_DIR/cmd/hearsay/rsrc_windows_amd64.syso" "$ROOT_DIR/cmd/launcher/rsrc_windows_amd64.syso" "$ROOT_DIR/cmd/installer/rsrc_windows_amd64.syso"' RETURN

    echo "==> Generating Windows icon, DPI manifest, and version resources (.syso)..."
    generate_winres_syso "$ROOT_DIR/cmd/hearsay/rsrc" "Hearsay" "Hearsay Multi-Stream TTS Chat Reader" "Hearsay.exe"
    generate_winres_syso "$ROOT_DIR/cmd/launcher/rsrc" "Hearsay" "Hearsay Portable Single-File Launcher" "Hearsay.exe"
    generate_winres_syso "$ROOT_DIR/cmd/installer/rsrc" "Hearsay Setup" "Hearsay Guided Setup Installer" "Hearsay-Setup.exe"

    echo "==> Building hearsay-core.exe v${VERSION} (CGO + MinGW)..."
    CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
        go build -trimpath -ldflags="-H windowsgui -s -w -X github.com/mrpoundsign/Hearsay/internal/ui.AppVersion=v${VERSION} -extldflags '-static-libgcc'" \
        -o "$BIN_DIR/hearsay-core.exe" ./cmd/hearsay
    echo "✓ Built $BIN_DIR/hearsay-core.exe"

    echo "==> Packing compressed Windows payload for single-file launcher & setup installer..."
    rm -rf "$launcher_stage" "$inst_stage"
    mkdir -p "$launcher_stage" "$inst_stage"
    cp -f "$BIN_DIR/hearsay-core.exe" "$launcher_stage/"
    cp -f "$BIN_DIR/onnxruntime.dll" "$launcher_stage/"
    cp -f "$BIN_DIR/sherpa-onnx-c-api.dll" "$launcher_stage/"
    cp -f "$BIN_DIR/sherpa-onnx-cxx-api.dll" "$launcher_stage/"
    pack_payload "$launcher_stage" "$payload_dir"
    rm -rf "$launcher_stage"

    cp -f "$BIN_DIR/hearsay-core.exe" "$inst_stage/"
    cp -f "$BIN_DIR/onnxruntime.dll" "$inst_stage/"
    cp -f "$BIN_DIR/sherpa-onnx-c-api.dll" "$inst_stage/"
    cp -f "$BIN_DIR/sherpa-onnx-cxx-api.dll" "$inst_stage/"
    [ -f "$ROOT_DIR/LICENSE" ] && cp -f "$ROOT_DIR/LICENSE" "$inst_stage/"
    [ -f "$ROOT_DIR/THIRDPARTY_NOTICES.md" ] && cp -f "$ROOT_DIR/THIRDPARTY_NOTICES.md" "$inst_stage/"
    pack_payload "$inst_stage" "$inst_payload_dir"
    rm -rf "$inst_stage"

    echo "==> Building standalone Hearsay.exe v${VERSION}..."
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
        go build -trimpath -ldflags="-H windowsgui -s -w -X main.AppVersion=${VERSION}" \
        -o "$BIN_DIR/Hearsay.exe" ./cmd/launcher

    echo "==> Building Fyne GUI installer Hearsay-Setup.exe v${VERSION} (-tags nosherpa)..."
    CC=x86_64-w64-mingw32-gcc CGO_ENABLED=1 GOOS=windows GOARCH=amd64 \
        go build -tags nosherpa -trimpath -ldflags="-H windowsgui -s -w -X main.AppVersion=${VERSION} -extldflags '-static-libgcc'" \
        -o "$BIN_DIR/Hearsay-Setup.exe" ./cmd/installer

    if command -v x86_64-w64-mingw32-objdump &>/dev/null; then
        echo "==> Verifying Hearsay-Setup.exe DLL Dependencies..."
        x86_64-w64-mingw32-objdump -p "$BIN_DIR/Hearsay-Setup.exe" | grep "DLL Name"
    fi

    clean_payload_dir "$payload_dir"
    clean_payload_dir "$inst_payload_dir"
    rm -f "$core_syso" "$launcher_syso" "$installer_syso"
    trap - RETURN

    echo "✓ Successfully built standalone single-file $BIN_DIR/Hearsay.exe and $BIN_DIR/Hearsay-Setup.exe"
}

build_linux() {
    fetch_linux_libs

    echo "==> Building hearsay-core v${VERSION} (Linux amd64 CGO)..."
    CGO_ENABLED=1 GOOS=linux GOARCH=amd64 \
        go build -trimpath -ldflags="-s -w -X github.com/mrpoundsign/Hearsay/internal/ui.AppVersion=v${VERSION} -extldflags '-Wl,-rpath,\$ORIGIN'" \
        -o "$BIN_DIR/hearsay-core" ./cmd/hearsay
    echo "✓ Built $BIN_DIR/hearsay-core"

    echo "==> Packing compressed Linux payload for single-file launcher..."
    local payload_dir="$ROOT_DIR/cmd/launcher/payload"
    local stage_dir="$BUILD_DIR/launcher_payload_stage_linux"
    trap 'rm -rf "$BUILD_DIR/launcher_payload_stage_linux"; clean_payload_dir "$ROOT_DIR/cmd/launcher/payload"' RETURN
    rm -rf "$stage_dir"
    mkdir -p "$stage_dir"
    cp -f "$BIN_DIR/hearsay-core" "$stage_dir/"
    cp -f "$BIN_DIR/libonnxruntime.so" "$stage_dir/"
    cp -f "$BIN_DIR/libsherpa-onnx-c-api.so" "$stage_dir/"
    cp -f "$BIN_DIR/libsherpa-onnx-cxx-api.so" "$stage_dir/"
    pack_payload "$stage_dir" "$payload_dir"
    rm -rf "$stage_dir"

    echo "==> Building standalone hearsay v${VERSION} (Linux amd64)..."
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
        go build -trimpath -ldflags="-s -w -X main.AppVersion=${VERSION}" \
        -o "$BIN_DIR/hearsay" ./cmd/launcher
    chmod +x "$BIN_DIR/hearsay"

    clean_payload_dir "$payload_dir"
    trap - RETURN

    echo "✓ Successfully built standalone single-file $BIN_DIR/hearsay"
}

build_mac() {
    fetch_mac_libs

    local arch
    arch="$(uname -m)"
    local goarch="arm64"
    if [ "$arch" = "x86_64" ]; then
        goarch="amd64"
    fi

    echo "==> Building hearsay-core v${VERSION} (macOS ${goarch} CGO)..."
    CGO_ENABLED=1 GOOS=darwin GOARCH=${goarch} \
        go build -trimpath -ldflags="-s -w -X github.com/mrpoundsign/Hearsay/internal/ui.AppVersion=v${VERSION} -extldflags '-Wl,-rpath,@executable_path'" \
        -o "$BIN_DIR/hearsay-core" ./cmd/hearsay
    echo "✓ Built $BIN_DIR/hearsay-core"

    echo "==> Packing compressed macOS payload for single-file launcher..."
    local payload_dir="$ROOT_DIR/cmd/launcher/payload"
    local stage_dir="$BUILD_DIR/launcher_payload_stage_mac"
    trap 'rm -rf "$BUILD_DIR/launcher_payload_stage_mac"; clean_payload_dir "$ROOT_DIR/cmd/launcher/payload"' RETURN
    rm -rf "$stage_dir"
    mkdir -p "$stage_dir"
    cp -f "$BIN_DIR/hearsay-core" "$stage_dir/"
    cp -f "$BIN_DIR/libonnxruntime.dylib" "$stage_dir/"
    cp -f "$BIN_DIR/libsherpa-onnx-c-api.dylib" "$stage_dir/"
    cp -f "$BIN_DIR/libsherpa-onnx-cxx-api.dylib" "$stage_dir/"
    pack_payload "$stage_dir" "$payload_dir"
    rm -rf "$stage_dir"

    echo "==> Building standalone hearsay v${VERSION} (macOS ${goarch})..."
    CGO_ENABLED=0 GOOS=darwin GOARCH=${goarch} \
        go build -trimpath -ldflags="-s -w -X main.AppVersion=${VERSION}" \
        -o "$BIN_DIR/hearsay" ./cmd/launcher
    chmod +x "$BIN_DIR/hearsay"

    clean_payload_dir "$payload_dir"
    trap - RETURN

    echo "✓ Successfully built standalone single-file $BIN_DIR/hearsay"
}

package_mac() {
    if [ ! -f "$BIN_DIR/hearsay-core" ]; then
        build_mac
    fi

    local arch
    arch="$(uname -m)"
    local pkg_arch="arm64"
    if [ "$arch" = "x86_64" ]; then
        pkg_arch="amd64"
    fi

    local tar_file="$DIST_DIR/Hearsay-v${VERSION}-darwin-${pkg_arch}.tar.gz"
    echo "==> Packaging macOS (${pkg_arch}) Hearsay.app bundle into $tar_file..."
    rm -f "$tar_file"

    local staging="$BUILD_DIR/package_staging_mac"
    local app_dir="$staging/Hearsay.app"
    local contents_dir="$app_dir/Contents"
    local macos_dir="$contents_dir/MacOS"
    local res_dir="$contents_dir/Resources"

    rm -rf "$staging"
    mkdir -p "$macos_dir" "$res_dir"

    cp -f "$BIN_DIR/hearsay-core" "$macos_dir/Hearsay"
    chmod +x "$macos_dir/Hearsay"
    cp -f "$BIN_DIR/libonnxruntime.dylib" "$macos_dir/"
    cp -f "$BIN_DIR/libsherpa-onnx-c-api.dylib" "$macos_dir/"
    cp -f "$BIN_DIR/libsherpa-onnx-cxx-api.dylib" "$macos_dir/"

    if [ -f "$ROOT_DIR/assets/icon.png" ] && command -v sips &>/dev/null && command -v iconutil &>/dev/null; then
        local iconset="$BUILD_DIR/Hearsay.iconset"
        rm -rf "$iconset"
        mkdir -p "$iconset"
        sips -z 16 16     "$ROOT_DIR/assets/icon.png" --out "$iconset/icon_16x16.png" >/dev/null 2>&1 || true
        sips -z 32 32     "$ROOT_DIR/assets/icon.png" --out "$iconset/icon_16x16@2x.png" >/dev/null 2>&1 || true
        sips -z 32 32     "$ROOT_DIR/assets/icon.png" --out "$iconset/icon_32x32.png" >/dev/null 2>&1 || true
        sips -z 64 64     "$ROOT_DIR/assets/icon.png" --out "$iconset/icon_32x32@2x.png" >/dev/null 2>&1 || true
        sips -z 128 128   "$ROOT_DIR/assets/icon.png" --out "$iconset/icon_128x128.png" >/dev/null 2>&1 || true
        sips -z 256 256   "$ROOT_DIR/assets/icon.png" --out "$iconset/icon_128x128@2x.png" >/dev/null 2>&1 || true
        sips -z 256 256   "$ROOT_DIR/assets/icon.png" --out "$iconset/icon_256x256.png" >/dev/null 2>&1 || true
        sips -z 512 512   "$ROOT_DIR/assets/icon.png" --out "$iconset/icon_256x256@2x.png" >/dev/null 2>&1 || true
        iconutil -c icns "$iconset" -o "$res_dir/icon.icns" >/dev/null 2>&1 || true
        rm -rf "$iconset"
    elif [ -f "$ROOT_DIR/assets/icon.png" ]; then
        cp "$ROOT_DIR/assets/icon.png" "$res_dir/icon.png"
    fi

    cat > "$contents_dir/Info.plist" <<EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>CFBundleExecutable</key>
    <string>Hearsay</string>
    <key>CFBundleIconFile</key>
    <string>icon.icns</string>
    <key>CFBundleIdentifier</key>
    <string>com.github.mrpoundsign.hearsay</string>
    <key>CFBundleName</key>
    <string>Hearsay</string>
    <key>CFBundleDisplayName</key>
    <string>Hearsay</string>
    <key>CFBundlePackageType</key>
    <string>APPL</string>
    <key>CFBundleShortVersionString</key>
    <string>${VERSION}</string>
    <key>CFBundleVersion</key>
    <string>${VERSION}</string>
    <key>LSMinimumSystemVersion</key>
    <string>11.0</string>
    <key>NSHighResolutionCapable</key>
    <true/>
</dict>
</plist>
EOF

    if command -v codesign &>/dev/null; then
        codesign --force --deep --sign - "$app_dir" 2>/dev/null || true
    fi

    [ -f "$ROOT_DIR/THIRDPARTY_NOTICES.md" ] && cp "$ROOT_DIR/THIRDPARTY_NOTICES.md" "$staging/"
    [ -f "$ROOT_DIR/LICENSE" ] && cp "$ROOT_DIR/LICENSE" "$staging/"
    [ -f "$ROOT_DIR/FyneApp.toml" ] && cp "$ROOT_DIR/FyneApp.toml" "$staging/"
    (cd "$staging" && tar -czf "$tar_file" .)
    rm -rf "$staging"
    echo "✓ Successfully packaged $tar_file (containing Hearsay.app)"
}

package_dist() {
    if [ ! -f "$BIN_DIR/Hearsay.exe" ]; then
        build_win
    fi
    if [ ! -f "$BIN_DIR/hearsay" ]; then
        build_linux
    fi

    local zip_file="$DIST_DIR/Hearsay-v${VERSION}-windows-amd64.zip"
    echo "==> Packaging Windows distribution into $zip_file..."
    rm -f "$zip_file"

    local staging="$BUILD_DIR/package_staging"
    rm -rf "$staging"
    mkdir -p "$staging"

    cp "$BIN_DIR/Hearsay.exe" "$staging/"
    [ -f "$ROOT_DIR/THIRDPARTY_NOTICES.md" ] && cp "$ROOT_DIR/THIRDPARTY_NOTICES.md" "$staging/"
    [ -f "$ROOT_DIR/LICENSE" ] && cp "$ROOT_DIR/LICENSE" "$staging/"
    [ -f "$ROOT_DIR/FyneApp.toml" ] && cp "$ROOT_DIR/FyneApp.toml" "$staging/"

    if command -v zip &>/dev/null; then
        (cd "$staging" && zip -q -r "$zip_file" .)
    else
        python3 -c "
import os, zipfile
with zipfile.ZipFile('$zip_file', 'w', zipfile.ZIP_DEFLATED) as zf:
    for root, _, files in os.walk('$staging'):
        for file in files:
            p = os.path.join(root, file)
            zf.write(p, os.path.relpath(p, '$staging'))
"
    fi
    rm -rf "$staging"
    echo "✓ Successfully packaged $zip_file"

    if [ -f "$BIN_DIR/Hearsay-Setup.exe" ]; then
        local setup_file="$DIST_DIR/Hearsay-v${VERSION}-windows-amd64-setup.exe"
        cp -f "$BIN_DIR/Hearsay-Setup.exe" "$setup_file"
        echo "✓ Successfully packaged $setup_file"
    fi

    local tar_file="$DIST_DIR/Hearsay-v${VERSION}-linux-amd64.tar.gz"
    echo "==> Packaging Linux distribution into $tar_file..."
    rm -f "$tar_file"
    mkdir -p "$staging"
    cp "$BIN_DIR/hearsay" "$staging/"
    chmod +x "$staging/hearsay"
    [ -f "$ROOT_DIR/THIRDPARTY_NOTICES.md" ] && cp "$ROOT_DIR/THIRDPARTY_NOTICES.md" "$staging/"
    [ -f "$ROOT_DIR/LICENSE" ] && cp "$ROOT_DIR/LICENSE" "$staging/"
    [ -f "$ROOT_DIR/FyneApp.toml" ] && cp "$ROOT_DIR/FyneApp.toml" "$staging/"
    (cd "$staging" && tar -czf "$tar_file" .)
    rm -rf "$staging"
    echo "✓ Successfully packaged $tar_file"

    echo "==> Generating release checksums..."
    (
        cd "$DIST_DIR"
        rm -f checksums.txt SHA256SUMS.txt
        TMP_SUMS="$(mktemp)"
        for f in *; do
            [ -f "$f" ] || continue
            sha256sum "$f" >> "$TMP_SUMS"
        done
        sort -u -k2 -o checksums.txt "$TMP_SUMS"
        rm -f "$TMP_SUMS"
        cp -f checksums.txt SHA256SUMS.txt
    )
}

clean() {
    echo "==> Cleaning build artifacts..."
    rm -rf "$BUILD_DIR" "$DIST_DIR"
    echo "✓ Clean complete."
}

case "$TARGET" in
    all)
        run_lint
        run_test
        build_win
        build_linux
        package_dist
        ;;
    win)
        build_win
        ;;
    linux)
        build_linux
        ;;
    mac|osx)
        build_mac
        ;;
    lint)
        run_lint
        ;;
    tidy)
        go mod tidy
        ;;
    test)
        run_test
        ;;
    package)
        package_dist
        ;;
    package-mac)
        package_mac
        ;;
    fetch-dlls)
        fetch_dlls
        ;;
    clean)
        clean
        ;;
    *)
        echo "Unknown target: $TARGET"
        echo "Valid targets: all, win, linux, mac, lint, test, package, package-mac, fetch-dlls, clean"
        exit 1
        ;;
esac
