#!/usr/bin/env bash
# Downloads the prebuilt OR-Tools C++ bundle for Windows (MSVC / Visual Studio 2022)
# and builds our C++ binding (bind.cpp) into a self-contained DLL (ortools_csat.dll)
# which the Go OR-Tools solver (build tag `ortools`, Windows) loads at runtime via
# syscall.LoadLibrary. This avoids cgo entirely on Windows.
#
# Usage (Windows runner, after VS + LLVM/clang-cl are available):
#   bash scripts/build_ortools_windows.sh

set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WIN_DIR="$ROOT/third_party/ortools_win"
BIND_DIR="$ROOT/third_party/ortools_bind"

# OR-Tools release tag + asset (VS2022 C++ build => ortools.lib + ortools.dll).
OR_URL="https://github.com/google/or-tools/releases/download/v9.11/or-tools_x64_VisualStudio2022_cpp_v9.11.4210.zip"

mkdir -p "$ROOT/third_party" "$WIN_DIR"

if [ ! -d "$WIN_DIR/include" ]; then
  echo ">> Downloading OR-Tools 9.11 (VS2022) C++ ..."
  tmp="$(mktemp -d)"
  curl -fL "$OR_URL" -o "$tmp/ortools.zip"

  win_tmp="$(cygpath -w "$tmp")"
  powershell -NoProfile -Command "Expand-Archive -Path '$win_tmp\\ortools.zip' -DestinationPath '$win_tmp\\extract'"

  # Locate the extracted root that contains include/ lib/ bin/
  src="$(find "$tmp/extract" -maxdepth 2 -type d -name include | head -1 | xargs dirname)"
  cp -r "$src"/. "$WIN_DIR"/
  rm -rf "$tmp"
fi

echo ">> Building ortools_csat.dll (C++ binding + OR-Tools) with clang-cl ..."
export MSYS_NO_PATHCONV=1
WIN_DIR_W="$(cygpath -w "$WIN_DIR" | tr '\\' '/')"
BIND_DIR_W="$(cygpath -w "$BIND_DIR" | tr '\\' '/')"

# OR-Tools depends on Abseil/Protobuf/etc. Link against every .lib shipped in
# third_party/ortools_win/lib. Repeat the list so circular back-references
# between ortools.lib and its dependencies resolve.
DEPS="$(ls "$WIN_DIR/lib"/*.lib 2>/dev/null | grep -v "/ortools.lib" | sed 's#.*/##' | tr '\n' ' ')"
echo ">> Linking with deps: $DEPS"

clang-cl /std:c++20 /EHsc /MD "/I$WIN_DIR_W/include" "/I$BIND_DIR_W" /LD \
  "$(cygpath -w "$BIND_DIR/bind.cpp" | tr '\\' '/')" \
  "/Fe$WIN_DIR_W/ortools_csat.dll" \
  "/link" "/LIBPATH:$WIN_DIR_W/lib" ortools.lib $DEPS ortools.lib $DEPS

echo ">> Done. ortools_csat.dll (+ ortools.dll) ready under third_party/ortools_win."
