#!/usr/bin/env sh
set -eu

GO_VERSION="${1:-1.26.6}"
NODE_MAJOR="${2:-20}"
NODE_VERSION="${NODE_VERSION:-20.20.2}"
CMAKE_VERSION="${CMAKE_VERSION:-3.31.8}"
DOCKER_CLI_VERSION="${DOCKER_CLI_VERSION:-29.4.3}"
PYTHON_STANDALONE_URL="${PYTHON_STANDALONE_URL:-https://github.com/astral-sh/python-build-standalone/releases/download/20260623/cpython-3.12.13%2B20260623-x86_64-unknown-linux-gnu-install_only.tar.gz}"

install_go() {
  arch="$(uname -m)"
  [ "$arch" = "x86_64" ] || { echo "unsupported arch: $arch" >&2; exit 1; }
  curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tgz
  rm -rf /usr/local/go
  tar -C /usr/local -xzf /tmp/go.tgz
  rm -f /tmp/go.tgz
}

install_docker_cli() {
  arch="$(uname -m)"
  [ "$arch" = "x86_64" ] || { echo "unsupported arch: $arch" >&2; exit 1; }
  curl -fsSL "https://download.docker.com/linux/static/stable/x86_64/docker-${DOCKER_CLI_VERSION}.tgz" -o /tmp/docker-cli.tgz
  tar -C /tmp -xzf /tmp/docker-cli.tgz docker/docker
  install -m 0755 /tmp/docker/docker /usr/local/bin/docker
  rm -rf /tmp/docker /tmp/docker-cli.tgz
}

install_node_glibc217() {
  curl -fsSL "https://unofficial-builds.nodejs.org/download/release/v${NODE_VERSION}/node-v${NODE_VERSION}-linux-x64-glibc-217.tar.xz" -o /tmp/node.tar.xz
  rm -rf /usr/local/node
  mkdir -p /usr/local/node
  tar -C /usr/local/node --strip-components=1 -xf /tmp/node.tar.xz
  ln -sf /usr/local/node/bin/node /usr/local/bin/node
  ln -sf /usr/local/node/bin/npm /usr/local/bin/npm
  ln -sf /usr/local/node/bin/npx /usr/local/bin/npx
  rm -f /tmp/node.tar.xz
}

install_cmake_binary() {
  curl -fsSL "https://github.com/Kitware/CMake/releases/download/v${CMAKE_VERSION}/cmake-${CMAKE_VERSION}-linux-x86_64.tar.gz" -o /tmp/cmake.tgz
  rm -rf /usr/local/cmake
  mkdir -p /usr/local/cmake
  tar -C /usr/local/cmake --strip-components=1 -xzf /tmp/cmake.tgz
  ln -sf /usr/local/cmake/bin/cmake /usr/local/bin/cmake
  ln -sf /usr/local/cmake/bin/ctest /usr/local/bin/ctest
  rm -f /tmp/cmake.tgz
}

install_python_standalone() {
  curl -fsSL "$PYTHON_STANDALONE_URL" -o /tmp/python-standalone.tgz
  rm -rf /usr/local/python
  mkdir -p /usr/local/python
  tar -C /usr/local/python --strip-components=1 -xzf /tmp/python-standalone.tgz
  ln -sf /usr/local/python/bin/python3 /usr/local/bin/python3
  rm -f /tmp/python-standalone.tgz
}

if command -v apt-get >/dev/null 2>&1; then
  apt-get update
  apt-get install -y --no-install-recommends \
    ca-certificates curl git tar gzip xz-utils build-essential cmake pkg-config \
    python3 openssl libssl-dev docker.io e2fsprogs
  glibc_version="$(getconf GNU_LIBC_VERSION 2>/dev/null | awk '{print $2}')"
  if [ "$glibc_version" = "2.27" ]; then
    install_python_standalone
    install_node_glibc217
    install_cmake_binary
  else
    curl -fsSL "https://deb.nodesource.com/setup_${NODE_MAJOR}.x" | bash -
    apt-get install -y --no-install-recommends nodejs
  fi
  rm -rf /var/lib/apt/lists/*
elif command -v dnf >/dev/null 2>&1; then
  dnf install -y --allowerasing \
    ca-certificates curl git tar gzip xz gcc gcc-c++ make cmake pkgconf-pkg-config \
    python3 python3.12 openssl openssl-devel findutils shadow-utils
  ln -sf /usr/bin/python3.12 /usr/local/bin/python3
  dnf install -y e2fsprogs || true
  curl -fsSL "https://rpm.nodesource.com/setup_${NODE_MAJOR}.x" | bash -
  dnf install -y nodejs
  install_docker_cli
  dnf clean all
else
  echo "unsupported base image: expected apt-get or dnf" >&2
  exit 1
fi

install_go
go version
node --version
npm --version
