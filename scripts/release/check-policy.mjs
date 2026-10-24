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

assert.deepEqual(Object.keys(linux.jobs), ['linux'], 'tags must not allocate optional platform runners');
assert.ok(linux.on.push.tags.includes('*'));
assert.equal(linux.jobs.linux.steps[0].with.ref, '${{ inputs.release_tag || github.ref }}');
const automaticRelease = linux.jobs.linux.steps.find((step) => step.name === 'Release');
assert.equal(automaticRelease.with.target_commitish, undefined, 'existing pushed tags do not need a target override');
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
assert.equal(stages.length, 3);
assert.match(stages[0], /--platform=\$BUILDPLATFORM/);
assert.match(stages[1], /--platform=\$BUILDPLATFORM/);
assert.ok(stages[1].includes(`golang:${goVersion}-alpine `), 'Docker and go.mod must use the same Go patch');
assert.doesNotMatch(stages[2], /--platform=/, 'runtime must retain the requested target platform');
assert.match(dockerfile, /CGO_ENABLED=0/);
assert.match(dockerfile, /GOOS=\$\{TARGETOS:-linux\} GOARCH=\$\{TARGETARCH:-amd64\}/);
const build = images.jobs.build_single_arch.steps.find((step) => step.uses?.startsWith('docker/build-push-action'));
assert.equal(build.with['cache-to'], 'type=gha,mode=max,scope=release-${{ matrix.arch }}');
assert.ok(images.jobs.build_single_arch.steps.some((step) => step.run?.includes('scripts/release/verify-image.sh')));
for (const step of images.jobs.create_manifests.steps.filter((step) => step.name?.includes('latest'))) {
  assert.match(step.run, /\$\{TAG\}-amd64/);
  assert.match(step.run, /\$\{TAG\}-arm64/);
  assert.doesNotMatch(step.run, /:latest-(amd64|arm64)/, 'manifest inputs must belong to the same release');
}
assert.ok(main.on.push.branches.includes('main'));
assert.match(main.jobs.docker.steps.find((step) => step.uses?.startsWith('docker/build-push-action')).with.tags, /:sha-/);
console.log('Release policy passed: Linux defaults, manual optional artifacts, immutable sources, cross-compiled image targets.');
