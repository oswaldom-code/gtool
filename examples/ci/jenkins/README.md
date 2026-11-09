# Jenkins — running an app's component tests with gtool in CI

A local Jenkins that builds a sample app from its own git repository and runs
its component tests with `gtool test`, the way a CI/CD pipeline would. gtool is
installed on the agent like any other CLI; the pipeline belongs to the app
(`examples/postgres-app/Jenkinsfile`).

## Layout

```
ci/jenkins/
├── Dockerfile    # Jenkins LTS + Docker CLI + gtool + plugins + JCasC
├── plugins.txt   # configuration-as-code, job-dsl, workflow-aggregator, git, htmlpublisher
├── casc.yml      # admin user and the postgres-app pipeline job
└── run.sh        # up: build and start Jenkins / down: remove it and its data
```

## Run it

```bash
./run.sh up      # http://localhost:8090 (admin/admin)
# Build the "postgres-app" job; the Karate report is linked from each build.
./run.sh down
```

`run.sh up` copies `examples/postgres-app` into a standalone git repository
under `~/.gtool-jenkins/repos/` (the app's "own" repository) and points the job
at it. Commit to that repository and rebuild to see the pipeline react.

Variables: `DATA_DIR` (default `~/.gtool-jenkins`), `APP` (default
`postgres-app`), `HTTP_PORT` (default `8090`).

## Pipeline

1. **Checkout** of the app repository.
2. **Build image** — `docker build -t "$APP_IMAGE" .`, tagged per build
   (`postgres-app:ci-<build number>`); gtool picks it up from
   `GTOOL_DOCKER_IMAGE`.
3. **Component tests** — `gtool test --config component-config.yml`; a failing
   scenario makes gtool exit non-zero and fails the build. gtool always tears
   down the mocks and the app.
4. **Post** — removes the build's image and publishes
   `test/component/reports/karate-summary.html`, also for failed builds.

## Concurrent builds

Two builds of the job can run at the same time on one agent (`numExecutors: 2`):

- **Ports** — `component-config.yml` uses `port: auto` for the mock and
  `host-port: auto` for the app, so gtool picks free host ports per run. The
  app gets the mock port through `${mocks.postgresql.port}`, and the features
  reach the app through `appUrl`.
- **Containers** — `GTOOL_RUN_ID=$BUILD_TAG` labels and names every container
  of the run, so one build's teardown never touches another's.
- **Image** — each build tags its own image instead of sharing
  `postgres-app:latest`.

## Agent requirements

gtool drives Docker and expects to reach what it starts on `localhost`. An
agent that talks to the host daemon through `/var/run/docker.sock` therefore
needs:

- **Host network** — mocks and the app publish their ports on the host; gtool's
  readiness checks and the Karate launcher use `localhost`.
- **Workspace at the same path on host and agent** — gtool bind-mounts the
  workspace's features and reports into the Karate container, and the host
  daemon resolves those paths. Here `JENKINS_HOME` is mounted at the same
  absolute path on both sides.
- **Docker socket access** — the agent user joins the socket's group.
- **The Karate launcher image** (`gtool/test-launcher-back:latest`) available
  to the daemon; in a shared CI it should come from a registry.

An agent running directly on a VM with Docker meets all of these without extra
setup.

## Demo-only settings

- `ALLOW_LOCAL_CHECKOUT` lets the job clone a `file://` repository.
- `DirectoryBrowserSupport.CSP` is cleared so the Karate HTML report renders
  with its scripts. Do not use either on a shared Jenkins.
