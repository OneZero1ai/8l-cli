# Releasing 8l

The `8l` operator CLI is released by pushing a `v*` tag. AWS CodeBuild
project `8l-cli-release` (account `124074140789`, region `us-east-1`)
picks up the tag via the `8th-layer-github` CodeConnections webhook,
runs `ci/buildspecs/cli-release.yml`, and publishes:

```
s3://8l-cli-releases-124074140789-us-east-1/8l/vX.Y.Z/
  ├── 8l_Darwin_arm64.tar.gz
  ├── 8l_Darwin_x86_64.tar.gz
  ├── 8l_Linux_arm64.tar.gz
  ├── 8l_Linux_x86_64.tar.gz
  ├── 8l_Windows_arm64.zip
  ├── 8l_Windows_x86_64.zip
  └── SHA256SUMS
```

Each Unix tarball contains a single `8l` binary at the root. The
Windows zips contain `8l.exe`. `SHA256SUMS` is the standard
`sha256sum`-compatible format and is consumed by the installer at
`install.8th-layer.ai` to verify each artifact before extracting.

## Cutting a release

```sh
# 1. Tag and push.
git tag v0.1.0
git push origin v0.1.0

# 2. Watch the build.
aws codebuild list-builds-for-project \
  --project-name 8l-cli-release \
  --profile 8th-layer-app --region us-east-1 | head

# 3. Verify the artifacts.
aws s3 ls s3://8l-cli-releases-124074140789-us-east-1/8l/v0.1.0/ \
  --profile 8th-layer-app --region us-east-1
```

## Manual fallback

If CodeBuild is unavailable, reproduce the publish locally:

```sh
make release-tarballs                          # writes ./dist with the same layout
aws s3 cp dist/ s3://8l-cli-releases-124074140789-us-east-1/8l/v0.1.0/ \
  --recursive --profile 8th-layer-app --region us-east-1
```

Keep `Makefile`'s `release-tarballs` target and
`ci/buildspecs/cli-release.yml` in sync — the installer's checksum
verification will fail if the tarball naming or the binary location
inside the tarball diverges.
