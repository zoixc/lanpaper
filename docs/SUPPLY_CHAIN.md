# Verifying Lanpaper release images

Lanpaper's main-branch release job publishes a multi-platform image index plus
three independent supply-chain signals:

- an SPDX JSON and a CycloneDX JSON SBOM are generated for every CI build and
  retained as the `lanpaper-sbom` workflow artifact;
- BuildKit attaches an in-toto/SLSA provenance attestation and an image SBOM to
  each pushed Docker Hub image;
- Sigstore cosign keyless-signs the immutable multi-platform digest. No
  long-lived signing key is stored in GitHub or Docker Hub.

Tags are convenient selectors, not identities. Resolve a tag once and verify
and deploy its digest:

```sh
IMAGE=ptabi/lanpaper:latest
DIGEST="$(docker buildx imagetools inspect "$IMAGE" \
  --format '{{json .Manifest.Digest}}' | tr -d '"')"
test -n "$DIGEST"
REF="ptabi/lanpaper@$DIGEST"
```

## Verify the keyless signature

Install [cosign](https://docs.sigstore.dev/cosign/system_config/installation/),
then constrain both the GitHub Actions issuer and the exact trusted workflow:

```sh
# For an image selected from main:
cosign verify \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity \
    'https://github.com/zoixc/lanpaper/.github/workflows/ci.yml@refs/heads/main' \
  "$REF"

# For a semver release tag (replace 0.16.0 with the selected tag):
cosign verify \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity \
    'https://github.com/zoixc/lanpaper/.github/workflows/release.yml@refs/tags/0.16.0' \
  "$REF"
```

The command must report a verified Rekor transparency-log entry and the exact
expected certificate identity. `latest` is normally replaced by the tag release
workflow, while a main-branch image is signed by CI; resolve the digest and
choose the identity for the publication being verified. Do not weaken identity
to an organization-wide wildcard: another repository or workflow must not be
trusted to publish Lanpaper.

## Inspect SBOM and provenance attestations

Buildx can display the attestations attached to the image index:

```sh
docker buildx imagetools inspect "$REF"
docker buildx imagetools inspect "$REF" --format '{{json .SBOM}}' > sbom.json
docker buildx imagetools inspect "$REF" --format '{{json .Provenance}}' > provenance.json
```

Check that provenance identifies this repository, `.github/workflows/ci.yml`,
the intended commit and the expected Dockerfile/build arguments. SBOMs should
contain both the Go modules and Alpine packages in the shipped image. Treat a
missing attestation, an unexpected source commit, or an unverifiable signature
as a release failure.

The downloadable CI SBOM files are useful for scanners that do not understand
OCI attestations. Workflow artifacts are convenience copies; the digest-bound
registry attestations are authoritative for a deployed image.

## Pin deployment to the verified digest

After verification, replace the mutable tag in Compose with the immutable
reference:

```yaml
services:
  lanpaper:
    image: ptabi/lanpaper@sha256:REPLACE_WITH_VERIFIED_DIGEST
```

Repeat verification before changing that digest during an upgrade.
