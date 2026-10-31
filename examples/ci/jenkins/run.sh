#!/usr/bin/env bash
# Local Jenkins for trying gtool in CI.
#
#   ./run.sh up     build gtool, seed the app repository and start Jenkins
#   ./run.sh down   stop Jenkins and remove its data
#
# Jenkins drives the host Docker daemon, so two things must hold for gtool:
#   - host network: gtool reaches the mocks and the app on localhost;
#   - same path on host and container for JENKINS_HOME: gtool bind-mounts the
#     workspace's features and reports, and the host daemon resolves those paths.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
ROOT="$(cd "$HERE/../../.." && pwd)"
DATA_DIR="${DATA_DIR:-$HOME/.gtool-jenkins}"
APP="${APP:-postgres-app}"
NAME=gtool-jenkins
IMAGE=gtool-jenkins:local
HTTP_PORT="${HTTP_PORT:-8090}"

# seed_app_repo copies the example into a standalone git repository, the way
# the app would live in its own repository in a real setup.
seed_app_repo() {
  local repo="$DATA_DIR/repos/$APP"
  rm -rf "$repo"
  mkdir -p "$repo"
  (cd "$ROOT/examples/$APP" && git ls-files -z --cached --others --exclude-standard | xargs -0 cp --parents -t "$repo")
  git -C "$repo" init -q -b main
  git -C "$repo" add -A
  git -C "$repo" -c user.name=ci-demo -c user.email=ci-demo@localhost commit -q -m "import $APP"
  echo "$repo"
}

up() {
  trap 'rm -f "$HERE/gtool"' EXIT
  CGO_ENABLED=0 go build -C "$ROOT" -o "$HERE/gtool" ./cmd/gtool
  docker build -t "$IMAGE" "$HERE"

  mkdir -p "$DATA_DIR/home"
  local repo
  repo="$(seed_app_repo)"

  docker run -d --name "$NAME" --network host \
    --group-add "$(stat -c %g /var/run/docker.sock)" \
    -v /var/run/docker.sock:/var/run/docker.sock \
    -v "$DATA_DIR:$DATA_DIR" \
    -e JENKINS_HOME="$DATA_DIR/home" \
    -e APP_REPO_URL="file://$repo" \
    -e JENKINS_OPTS="--httpPort=$HTTP_PORT" \
    -e JAVA_OPTS="-Djenkins.install.runSetupWizard=false -Dhudson.plugins.git.GitSCM.ALLOW_LOCAL_CHECKOUT=true -Dhudson.model.DirectoryBrowserSupport.CSP=" \
    "$IMAGE" >/dev/null
  echo "Jenkins: http://localhost:$HTTP_PORT (admin/admin), job: $APP"
}

down() {
  docker rm -f "$NAME" >/dev/null 2>&1 || true
  rm -rf "$DATA_DIR"
}

case "${1:-}" in
  up) up ;;
  down) down ;;
  *) echo "usage: $0 up|down" >&2; exit 1 ;;
esac
