import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

const read = (name) => readFileSync(new URL(`../../${name}`, import.meta.url), 'utf8');
const workflow = (name) => Bun.YAML.parse(read(`.github/workflows/${name}.yml`));
const linux = workflow('release');
const optional = workflow('release-optional');
const images = workflow('docker-image-arm64');
const main = workflow('docker-publish');
const goVersion = read('go.mod').match(/^go (\d+\.\d+\.\d+)$/m)?.[1];
assert.ok(goVersion, 'Go builds must declare a patch version');
for (const name of ['release', 'release-optional', 'electron-build']) {
  const setups = Object.values(workflow(name).jobs).flatMap((job) => job.steps ?? [])
    .filter((step) => step.uses?.startsWith('actions/setup-go@'));
  assert.ok(setups.length, `${name} must set up Go explicitly`);
  for (const step of setups) {
    assert.equal(step.with['go-version-file'], 'go.mod');
    assert.equal(step.with['go-version'], undefined, 'do not override the fixed module toolchain');
  }
}

assert.deepEqual(Object.keys(linux.jobs), ['frontend', 'images', 'desktop', 'linux'], 'release must share one frontend before existing artifact jobs');
assert.equal(linux.jobs.linux.needs, 'frontend');
assert.equal(linux.jobs.images.uses, './.github/workflows/docker-image-arm64.yml');
assert.equal(linux.jobs.desktop.uses, './.github/workflows/electron-build.yml');
assert.equal(linux.jobs.frontend.uses, './.github/workflows/release-frontend.yml');
assert.ok(linux.on.push.tags.includes('*'));
assert.equal(linux.jobs.linux.steps[0].with.ref, '${{ inputs.release_tag || github.ref }}');
const automaticRelease = linux.jobs.linux.steps.find((step) => step.name === 'Release');
assert.match(automaticRelease.run, /gh release create/);
assert.match(automaticRelease.run, /--verify-tag/);
assert.doesNotMatch(automaticRelease.run, /--target|--clobber|release edit/, 'release creation must preserve existing tags and metadata');
const linuxFill = linux.jobs.linux.steps.find((step) => step.name === 'Fill missing Linux assets without updating release metadata').run;
assert.match(linuxFill, /'release', 'upload'/);
assert.doesNotMatch(linuxFill, /--clobber|'release', '(create|edit)'/);
assert.deepEqual(Object.keys(optional.on), ['workflow_dispatch'], 'optional binaries must remain manual-only');
assert.deepEqual(optional.on.workflow_dispatch.inputs.platform.options, ['windows', 'macos']);
assert.equal(optional.jobs.build.if, "needs.prepare.outputs.build_needed == 'true'");
assert.equal(optional.jobs.build.steps[0].with.ref, '${{ needs.prepare.outputs.revision }}');
const upload = optional.jobs.build.steps.find((step) => step.name === 'Add asset to existing release').run;
assert.match(upload, /gh release upload/);
assert.doesNotMatch(upload, /--clobber|release (create|edit)/, 'optional artifacts must not overwrite or republish a release');

const dockerfile = read('Dockerfile');
const stages = dockerfile.split('\n').filter((line) => line.startsWith('FROM '));
assert.equal(stages.length, 5);
assert.match(stages[0], /--platform=\$BUILDPLATFORM/);
assert.match(stages[3], /--platform=\$BUILDPLATFORM/);
assert.ok(stages[3].includes(`golang:${goVersion}-alpine `), 'Docker and go.mod must use the same Go patch');
assert.doesNotMatch(stages[4], /--platform=/, 'runtime must retain the requested target platform');
assert.match(dockerfile, /CGO_ENABLED=0/);
assert.match(dockerfile, /GOOS=\$\{TARGETOS:-linux\} GOARCH=\$\{TARGETARCH:-amd64\}/);
const build = images.jobs.build_single_arch.steps.find((step) => step.name === 'Build & push single-arch (GHCR + optional Docker Hub)');
assert.match(build.run, /docker buildx build --push --platform "\$TARGET_PLATFORM"/);
assert.match(build.run, /buildcache-\$\{TARGET_ARCH\}/);
assert.deepEqual(images.jobs.build_single_arch.strategy.matrix.include.map(({arch, runner}) => [arch, runner]), [
  ['amd64', 'ubuntu-latest'], ['arm64', 'ubuntu-24.04-arm'],
]);
assert.equal(images.jobs.create_manifests.permissions.packages, 'write');
assert.ok(images.jobs.build_single_arch.steps.some((step) => step.run?.includes('scripts/release/verify-image.sh')));
for (const step of images.jobs.create_manifests.steps.filter((step) => step.name?.includes('latest'))) {
  assert.match(step.run, /\$\{TAG\}-amd64/);
  assert.match(step.run, /\$\{TAG\}-arm64/);
  assert.doesNotMatch(step.run, /:latest-(amd64|arm64)/, 'manifest inputs must belong to the same release');
}
assert.ok(main.on.push.branches.includes('main'));
assert.match(main.jobs.docker.steps.find((step) => step.name === 'Build and push').run, /:sha-/);
for (const name of ['release', 'release-optional', 'release-policy', 'docker-publish', 'docker-image-arm64', 'verify-notes', 'electron-build', 'release-frontend', 'invoice-pdf']) {
  for (const step of Object.values(workflow(name).jobs).flatMap((job) => job.steps ?? [])) {
    if (!step.uses) continue;
    assert.match(step.uses, /^(actions\/[^@]+|oven-sh\/setup-bun)@[a-f0-9]{40}$/, `${name}: company Actions must use approved sources and full commit SHAs`);
  }
}

const electron = workflow('electron-build');
assert.equal(electron.on.workflow_dispatch.inputs.release_tag.required, true);
assert.equal(electron.jobs.build.steps[0].with.ref, '${{ inputs.release_tag || github.ref }}');
assert.equal(electron.jobs.release.permissions.contents, 'write');
assert.equal(electron.jobs.release.if, undefined, 'manual desktop backfills must upload too');
assert.equal(electron.jobs.build.steps.find(s => s.uses?.startsWith('actions/upload-artifact@')).with['if-no-files-found'], 'error');
const desktopUpload = electron.jobs.release.steps.find(s => s.name === 'Add desktop assets to existing Linux release').run;
assert.match(desktopUpload, /'release', 'upload'/);
assert.doesNotMatch(desktopUpload, /--clobber|'release', '(create|edit)'/);
assert.deepEqual(images.on.push.tags, ['*-alpha*', '!cn-tunnel-*', '!agent-workflow-*', '!project-decision-kit-*']);
assert.equal(linux.jobs.images.if, "github.event_name == 'push'", 'manual Linux backfill must not rebuild existing images');
assert.deepEqual(Object.keys(electron.on), ['workflow_call', 'workflow_dispatch']);
assert.match(images.jobs.create_manifests.steps.find(s => s.name === 'Publish same-commit SHA image').run, /:sha-\$\{BUILD_COMMIT:0:7\}/);
assert.equal(main.jobs.docker.if, "needs.prepare.outputs.release_build != 'true'");
assert.equal(main.jobs.prepare.steps[0].with['fetch-depth'], 2);
const sourceDeps = dockerfile.indexOf('RUN go mod download');
for (const arg of ['ARG BUILD_COMMIT', 'ARG BUILD_BRANCH', 'ARG BUILD_REPOSITORY']) {
  assert.ok(dockerfile.indexOf(arg) > sourceDeps, `${arg} must not invalidate Go dependency cache`);
}
assert.ok(dockerfile.indexOf('ARG APP_VERSION') > dockerfile.indexOf('RUN bun install'), 'version must not invalidate Bun dependency cache');
assert.match(dockerfile, /FROM \$\{FRONTEND_SOURCE\} AS frontend/);
assert.match(dockerfile, /COPY --from=frontend \/build\/dist/);
for (const name of ['release', 'docker-image-arm64', 'electron-build']) {
  const restore = Object.values(workflow(name).jobs).flatMap(j => j.steps ?? []).find(s => s.name === 'Restore verified frontend');
  assert.match(restore.run, /frontend-artifact.py restore/);
}
const frontendUpload = workflow('release-frontend').jobs.frontend.steps.find(s => s.name === 'Upload shared frontend');
assert.equal(frontendUpload.with['include-hidden-files'], true);
assert.deepEqual(frontendUpload.with.path.trim().split('\n'), ['.release-artifact/frontend.tar.gz', '.release-artifact/frontend.json']);
assert.equal(electron.jobs.release.steps[0].with.name, 'windows-build');
console.log('Release policy passed: Linux defaults, manual optional artifacts, immutable sources, cross-compiled image targets.');
